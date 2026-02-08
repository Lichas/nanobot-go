package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Lichas/nanobot-go/internal/agent"
	"github.com/Lichas/nanobot-go/internal/bus"
	"github.com/Lichas/nanobot-go/internal/config"
	"github.com/Lichas/nanobot-go/internal/cron"
	"github.com/Lichas/nanobot-go/internal/logging"
	"github.com/Lichas/nanobot-go/internal/providers"
	"github.com/spf13/cobra"
)

var (
	messageFlag   string
	sessionIDFlag string
)

func init() {
	agentCmd.Flags().StringVarP(&messageFlag, "message", "m", "", "Message to send to the agent")
	agentCmd.Flags().StringVarP(&sessionIDFlag, "session", "s", "cli:default", "Session ID")
}

// agentCmd Agent 命令
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Interact with the agent directly",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		if _, err := logging.Init(config.GetDataDir()); err != nil {
			fmt.Printf("⚠ logging init error: %v\n", err)
		}

		// 检查 API key
		apiKey := cfg.GetAPIKey("")
		apiBase := cfg.GetAPIBase("")
		if apiKey == "" {
			return fmt.Errorf("no API key configured. Set one in ~/.nanobot/config.json")
		}

		// 创建 Provider
		provider, err := providers.NewOpenAIProvider(apiKey, apiBase, cfg.Agents.Defaults.Model)
		if err != nil {
			return fmt.Errorf("failed to create provider: %w", err)
		}

		// 创建组件
		messageBus := bus.NewMessageBus(100)

		// 创建 Cron 服务（agent 模式下也需要，但不启动）
		storePath := filepath.Join(cfg.Agents.Defaults.Workspace, ".cron", "jobs.json")
		cronService := cron.NewService(storePath)

		agentLoop := agent.NewAgentLoop(
			messageBus,
			provider,
			cfg.Agents.Defaults.Workspace,
			cfg.Agents.Defaults.Model,
			cfg.Agents.Defaults.MaxToolIterations,
			cfg.Tools.Web.Search.APIKey,
			agent.BuildWebFetchOptions(cfg),
			cfg.Tools.Exec,
			cfg.Tools.RestrictToWorkspace,
			cronService,
		)

		if messageFlag != "" {
			// 单条消息模式
			ctx := context.Background()
			response, err := agentLoop.ProcessDirect(ctx, messageFlag, sessionIDFlag, "cli", "direct")
			if err != nil {
				return err
			}
			// 流式输出已实时显示，仅在有内容时打印
			if response != "" {
				fmt.Printf("\n%s %s\n", logo, response)
			}
		} else {
			// 交互模式
			fmt.Printf("%s Interactive mode (Ctrl+C to exit)\n\n", logo)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// 处理 Ctrl+C
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigChan
				cancel()
			}()

			reader := bufio.NewReader(os.Stdin)
			for {
				select {
				case <-ctx.Done():
					fmt.Println("\nGoodbye!")
					return nil
				default:
				}

				fmt.Print("You: ")
				input, err := reader.ReadString('\n')
				if err != nil {
					return err
				}

				input = input[:len(input)-1] // 移除换行符
				if input == "" {
					continue
				}

				response, err := agentLoop.ProcessDirect(ctx, input, sessionIDFlag, "cli", "direct")
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					continue
				}

				// 流式输出已实时显示，仅在有内容时打印
				if response != "" {
					fmt.Printf("\n%s %s\n\n", logo, response)
				} else {
					fmt.Println()
				}
			}
		}

		return nil
	},
}

# Providers

当前仅支持 **OpenAI 兼容 API**。如需使用 Anthropic / Gemini / Groq 等非兼容模型，请通过 OpenAI 兼容网关（如 OpenRouter、vLLM、LiteLLM、自建兼容代理）转发。

配置示例（OpenRouter）：

```json
{
  "providers": {
    "openrouter": {
      "apiKey": "YOUR_KEY",
      "apiBase": "https://openrouter.ai/api/v1"
    }
  },
  "agents": {
    "defaults": {
      "model": "openrouter/your-model"
    }
  }
}
```

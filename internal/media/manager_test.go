package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Lichas/maxclaw/internal/bus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQQResolverStagesRemoteImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-binary"))
	}))
	defer server.Close()

	resolver := NewQQResolver(t.TempDir(), server.Client())
	media := &bus.MediaAttachment{
		Type:     "image",
		URL:      server.URL + "/image.png",
		Filename: "image.png",
		MimeType: "image/png",
	}

	staged, err := resolver.Stage(context.Background(), media)
	require.NoError(t, err)
	require.NotNil(t, staged)
	assert.Equal(t, "image.png", staged.Filename)
	assert.Equal(t, "image/png", staged.MimeType)
	assert.FileExists(t, staged.LocalPath)
}

func TestTelegramResolverStagesFileIDToLocalPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/getFile"):
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
				"ok": true,
				"result": map[string]interface{}{
					"file_path": "photos/file_1.jpg",
				},
			}))
		case strings.Contains(r.URL.Path, "/file/bottoken/photos/file_1.jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("jpeg-binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := &TelegramResolver{
		rootDir:    t.TempDir(),
		token:      "token",
		apiBaseURL: server.URL,
		httpClient: server.Client(),
	}

	staged, err := resolver.Stage(context.Background(), &bus.MediaAttachment{
		Type:     "image",
		FileID:   "abc123",
		MimeType: "image/jpeg",
	})
	require.NoError(t, err)
	require.NotNil(t, staged)
	assert.Equal(t, "image/jpeg", staged.MimeType)
	assert.FileExists(t, staged.LocalPath)
	assert.Equal(t, "file_1.jpg", staged.Filename)
}

func TestManagerStageInboundAddsLocalPath(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-binary"))
	}))
	defer server.Close()

	manager := NewManager(tmpDir)
	manager.Register("qq", NewQQResolver(tmpDir, server.Client()))

	attachment := &bus.MediaAttachment{
		Type:     "image",
		URL:      server.URL + "/image.png",
		Filename: "image.png",
		MimeType: "image/png",
	}
	staged, err := manager.StageInbound(context.Background(), "qq", attachment)
	require.NoError(t, err)
	require.NotNil(t, staged)
	assert.FileExists(t, staged.LocalPath)
	assert.Equal(t, "image.png", staged.Filename)
}

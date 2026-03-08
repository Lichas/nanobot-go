package channels

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramBuildInboundMessageFromPhoto(t *testing.T) {
	ch := NewTelegramChannel(&TelegramConfig{Token: "token", Enabled: true})

	msg := ch.buildInboundMessage(telegramMessage{
		MessageID: 101,
		From: telegramUser{
			ID:       42,
			Username: "alice",
		},
		Chat: telegramChat{
			ID: 1001,
		},
		Photo: []telegramPhoto{
			{FileID: "small"},
			{FileID: "large"},
		},
	})

	require.NotNil(t, msg)
	assert.Equal(t, "[Image]", msg.Text)
	assert.Equal(t, "alice", msg.Sender)
	assert.Equal(t, "1001", msg.ChatID)
	require.NotNil(t, msg.Media)
	assert.Equal(t, "image", msg.Media.Type)
	assert.Equal(t, "large", msg.Media.FileID)
	assert.Equal(t, "image/jpeg", msg.Media.MimeType)
}

func TestTelegramBuildInboundMessageUsesCaptionAndDocumentMime(t *testing.T) {
	ch := NewTelegramChannel(&TelegramConfig{Token: "token", Enabled: true})

	msg := ch.buildInboundMessage(telegramMessage{
		MessageID: 102,
		From: telegramUser{
			ID: 7,
		},
		Chat: telegramChat{
			ID: 2002,
		},
		Caption: "diagram",
		Document: &telegramDocument{
			FileID:   "doc-image",
			FileName: "diagram.png",
			MimeType: "image/png",
		},
	})

	require.NotNil(t, msg)
	assert.Equal(t, "diagram", msg.Text)
	assert.Equal(t, "7", msg.Sender)
	require.NotNil(t, msg.Media)
	assert.Equal(t, "image", msg.Media.Type)
	assert.Equal(t, "doc-image", msg.Media.FileID)
	assert.Equal(t, "diagram.png", msg.Media.URL)
	assert.Equal(t, "image/png", msg.Media.MimeType)
}

func TestTelegramBuildInboundMessageDropsEmptyNonMediaMessage(t *testing.T) {
	ch := NewTelegramChannel(&TelegramConfig{Token: "token", Enabled: true})

	msg := ch.buildInboundMessage(telegramMessage{
		MessageID: 103,
		From: telegramUser{
			ID: 8,
		},
		Chat: telegramChat{
			ID: 2003,
		},
	})

	assert.Nil(t, msg)
}

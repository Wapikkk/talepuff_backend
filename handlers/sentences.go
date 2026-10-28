package handlers

import (
	"encoding/json"
	"strings"
	"talepuff_backend/dto"
)

const (
	maxSentences        = 3000
	maxSentenceRunes    = 1000
	maxSentencesJSONLen = 1 << 20
)

func encodeSentences(items []dto.Sentence) string {
	clean := make([]dto.Sentence, 0, len(items))
	for _, item := range items {
		text := truncateRunes(strings.TrimSpace(item.Text), maxSentenceRunes)
		if text == "" {
			continue
		}
		item.Text = text
		item.Start = max(item.Start, 0)
		item.End = max(item.End, item.Start)
		clean = append(clean, item)
		if len(clean) == maxSentences {
			break
		}
	}
	if len(clean) == 0 {
		return ""
	}
	raw, err := json.Marshal(clean)
	if err != nil || len(raw) > maxSentencesJSONLen {
		return ""
	}
	return string(raw)
}

func decodeSentences(stored string) []dto.Sentence {
	if stored == "" {
		return nil
	}
	var items []dto.Sentence
	if err := json.Unmarshal([]byte(stored), &items); err != nil {
		return nil
	}
	return items
}

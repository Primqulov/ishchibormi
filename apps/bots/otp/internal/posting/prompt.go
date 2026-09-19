package posting

// sendText uses plain text and UTF-16-aware chunks. User descriptions must
// neither become Telegram HTML/Markdown nor exceed its 4096-unit limit.
func (e *Engine) sendText(chat int64, text string) error {
	for _, part := range textChunks(text, 3500) {
		if err := e.say(chat, part); err != nil {
			return err
		}
	}
	return nil
}
func textChunks(text string, limit int) []string {
	var chunks []string
	start, units := 0, 0
	for i, r := range text {
		n := 1
		if r > 0xffff {
			n = 2
		}
		if units+n > limit {
			chunks = append(chunks, text[start:i])
			start = i
			units = 0
		}
		units += n
	}
	if start < len(text) {
		chunks = append(chunks, text[start:])
	}
	return chunks
}

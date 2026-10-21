package base

const textReserve = 512

// TextBudget adalah batas byte teks hasil tool: max - 512, minimal 512 (K9).
func TextBudget(maxResponseBytes int) int {
	b := maxResponseBytes - textReserve
	if b < textReserve {
		return textReserve
	}
	return b
}

// ProviderMaxBytes adalah MaxBytes untuk provider get-endpoint: max - min(2048, max/4).
func ProviderMaxBytes(maxResponseBytes int) int {
	r := maxResponseBytes / 4
	if r > 2048 {
		r = 2048
	}
	return maxResponseBytes - r
}

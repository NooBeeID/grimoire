package base

// RequestMeta adalah metadata request yang diteruskan handler ke usecase.
type RequestMeta struct{ IP, UserAgent string }

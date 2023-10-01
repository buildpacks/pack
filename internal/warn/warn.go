package warn

type Warn struct {
	Messages []string
}

func(warn *Warn) Add(mesage string) *Warn {
	warn.Messages = append(warn.Messages, mesage)
	return warn
}

func(warn *Warn) AddSlice(messages... string) *Warn {
	warn.Messages = append(warn.Messages, messages...)
	return warn
}

func(w *Warn) AddWarn(warn *Warn) *Warn {
	w.Messages = append(w.Messages, warn.Messages...)
	return w
}
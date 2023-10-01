package warn

type Warn struct {
	Messages []string
}

func (warn *Warn) Add(message string) {
	warn.Messages = append(warn.Messages, message)
}

func (warn *Warn) AddSlice(messages ...string) {
	warn.Messages = append(warn.Messages, messages...)
}

func (warn *Warn) AddWarn(w Warn) {
	warn.Messages = append(warn.Messages, w.Messages...)
}

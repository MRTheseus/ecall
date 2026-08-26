package eventhost

import "time"

type Dispatcher interface{}

type Event any

type SMSReceived struct {
	DevID   string
	Sender  string
	Content string
	Time    time.Time
}

type SMSSent struct {
	DevID     string
	TargetURI string
	Content   string
	Time      time.Time
}

type LocalNumberLearned struct {
	DevID  string
	IMSI   string
	Number string
}

type LogNotify struct {
	Level   string
	Message string
}

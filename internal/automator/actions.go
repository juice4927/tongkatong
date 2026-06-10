package automator

// CheckinAction 打卡动作类型（该包内的类型定义）
type CheckinAction string

const (
	MorningSignin    CheckinAction = "morning_signin"
	MorningSignout   CheckinAction = "morning_signout"
	AfternoonSignin  CheckinAction = "afternoon_signin"
	AfternoonSignout CheckinAction = "afternoon_signout"
)

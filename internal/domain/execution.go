package domain

type ExecutionCard struct {
	Key             string
	Kind            string
	Title           string
	JobExecutionIDs []string
	Summary         ExecutionSummary
}

type ExecutionSummary struct {
	TotalJobs  int
	Succeeded  int
	Failed     int
	InProgress int
	Waiting    int
}

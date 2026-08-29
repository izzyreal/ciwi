package server

import (
	"testing"

	"github.com/izzyreal/ciwi/internal/application"
)

func TestJobLogChangeAffectsSelectedExecution(t *testing.T) {
	if !jobLogChangeAffects(application.Change{Resync: true}, "job-1") {
		t.Fatal("resync should affect the log")
	}
	change := application.Change{Topics: []application.ChangeTopic{application.ChangeJobOutput}, JobExecutionIDs: []string{"job-2"}}
	if jobLogChangeAffects(change, "job-1") {
		t.Fatal("unrelated execution affected the log")
	}
	change.JobExecutionIDs = append(change.JobExecutionIDs, "job-1")
	if !jobLogChangeAffects(change, "job-1") {
		t.Fatal("selected execution did not affect the log")
	}
}

package executiondiagnosis

import (
	"github.com/izzyreal/ciwi/internal/protocol"
	"github.com/izzyreal/ciwi/internal/requirements"
)

// DiagnoseQueuedJob adapts the persisted execution representation to the
// transport-neutral capability matcher. Keeping this bridge outside
// requirements prevents presentation code from depending transitively on the
// execution protocol.
func DiagnoseQueuedJob(job protocol.JobExecution, agents []requirements.AgentSnapshot) *requirements.SchedulingDiagnosis {
	if reason := protocol.JobSchedulingBlockedReason(job); reason != "" {
		if protocol.IsPendingJobExecutionStatus(job.Status) {
			return &requirements.SchedulingDiagnosis{State: requirements.DiagnosisWaiting, Summary: reason}
		}
		return nil
	}
	if !protocol.IsQueuedJobExecutionStatus(job.Status) {
		return nil
	}
	if protocol.IsJobWaitingForPrerequisites(job) {
		return nil
	}
	diagnosis := requirements.DiagnoseScheduling(job.RequiredCapabilities, agents)
	return &diagnosis
}

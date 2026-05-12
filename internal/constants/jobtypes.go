package constants

// Worker job_type constants — see ADR-0002.
//
// `job_type` is the discriminator the queue worker uses to route a queued
// row to the right delivery path. Per ADR-0002 it is a first-class
// VARCHAR(64) column on the messages table (not an enum), and the Go-side
// constants set below is the source of truth for what values are valid.
//
// To add a new job type:
//  1. Add a constant here.
//  2. Add a case in the worker dispatch switch.
//  3. (Optional) Add a corresponding queued-payload Go struct.
// No schema migration is required.
const (
	// JobTypeProcessOutbound is the default dispatch path for ERP-originated
	// business documents that need to be signed, encrypted, and POSTed as
	// a FideX envelope to the destination partner's message endpoint.
	JobTypeProcessOutbound = "process_outbound"

	// JobTypeSendJMDN dispatches the queue worker to build and POST a
	// signed J-MDN disposition notification back to a sender whose
	// inbound message we have just accepted. See spec §7.
	JobTypeSendJMDN = "send_jmdn"
)

// IsKnownJobType reports whether the given string is one of the job types
// the worker currently knows how to dispatch. Unknown job_type values are
// treated as a dead-letter condition by the dispatcher; this helper lets
// the enqueue path reject them up front rather than at delivery time.
func IsKnownJobType(jobType string) bool {
	switch jobType {
	case JobTypeProcessOutbound, JobTypeSendJMDN:
		return true
	default:
		return false
	}
}

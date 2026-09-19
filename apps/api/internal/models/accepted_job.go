package models

// AcceptedJobDetails records the details shared with the accepted applicant.
// Queue retries must retain the schedule and contact from this decision.
type AcceptedJobDetails struct {
	Work         ListingWorkDetails `bson:"work"`
	EmployerName string             `bson:"employerName"`
	ContactPhone string             `bson:"contactPhone"`
}

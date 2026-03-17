package main

// NATS event subjects published by jobs-api to the JOBS stream.
const (
	EventJobCreated     = "jobs.job.created"
	EventJobUpdated     = "jobs.job.updated"
	EventProfileCreated = "jobs.profile.created"
	EventProfileUpdated = "jobs.profile.updated"
)

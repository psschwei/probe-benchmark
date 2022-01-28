package pod

import "time"

type Stats struct {
	Created           time.Time
	Scheduled         time.Time
	Initialized       time.Time
	ContainersStarted time.Time
	ContainersReady   time.Time
	Ready             time.Time

	HasIP  time.Time
	Probed time.Time
}

func (s Stats) TimeToScheduled() time.Duration {
	return s.Scheduled.Sub(s.Created)
}

func (s Stats) TimeToInitialized() time.Duration {
	return s.Initialized.Sub(s.Created)
}

func (s Stats) TimeToContainersStarted() time.Duration {
	return s.ContainersStarted.Sub(s.Created)
}

func (s Stats) TimeToReady() time.Duration {
	return s.Ready.Sub(s.Created)
}

func (s Stats) TimeToIP() time.Duration {
	return s.HasIP.Sub(s.Created)
}

func (s Stats) TimeToProbed() time.Duration {
	return s.Probed.Sub(s.Created)
}

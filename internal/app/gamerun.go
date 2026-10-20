package app

import "time"

// GameRun is a running game of an environment (10 §3.1).
type GameRun struct {
	PID     int
	Exe     string
	Started time.Time
}

// GameRunning reports the environment's running game, if any.
func (a *App) GameRunning() (*GameRun, error) {
	e, err := a.Env()
	if err != nil {
		return nil, err
	}
	if a.GameProbe != nil {
		return a.GameProbe(e.GameRoot), nil
	}
	return gameProcess(e.GameRoot), nil
}

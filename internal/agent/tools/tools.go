// Package tools holds the functions Ziggy can call.
package tools

import (
	"context"
	"time"

	"github.com/jpatters/home-calendar/internal/agent"
	"github.com/jpatters/home-calendar/internal/gcal"
	"github.com/jpatters/home-calendar/internal/types"
)

type EventSource interface {
	Events() []types.Event
}

// CalendarWriter is satisfied by *gcal.Client.
type CalendarWriter interface {
	CreateEvent(ctx context.Context, calID string, e gcal.NewEvent) (gcal.Event, error)
	FindEvent(ctx context.Context, calID, title string, from, to time.Time) (gcal.Event, error)
	UpdateEvent(ctx context.Context, calID, eventID string, patch gcal.EventPatch) (gcal.Event, error)
	DeleteEvent(ctx context.Context, calID, eventID string) error
}

type HockeySource interface {
	Snapshot() *types.HockeySnapshot
}

type BaseballSource interface {
	Snapshot() *types.BaseballSnapshot
}

type WeatherSource interface {
	Snapshot() *types.WeatherSnapshot
}

// HotTubControl is satisfied by *hottub.Fetcher.
type HotTubControl interface {
	Snapshot() *types.HotTubSnapshot
	SetTarget(ctx context.Context, h types.HotTub, targetF float64) (*types.HotTubSnapshot, error)
}

// PoolControl is satisfied by *pool.Fetcher.
type PoolControl interface {
	Snapshot() *types.PoolSnapshot
	SetTarget(ctx context.Context, p types.Pool, targetC float64) (*types.PoolSnapshot, error)
}

// Deps are the data sources and devices the tools act on. Calendar is nil
// when no Google service account is configured. Location is the zone that
// times without an offset are interpreted in; nil means time.Local.
type Deps struct {
	Config   func() types.Config
	Location *time.Location
	Events   EventSource
	Calendar CalendarWriter
	Hockey   HockeySource
	Baseball BaseballSource
	Weather  WeatherSource
	HotTub   HotTubControl
	Pool     PoolControl
}

// All returns every tool, in the order they are offered to the model. The
// calendar write tools are left out when there is no Google client, so the
// model never offers changes it cannot make.
func All(d Deps) []agent.Tool {
	if d.Location == nil {
		d.Location = time.Local
	}
	all := []agent.Tool{
		listEvents(d),
	}
	if d.Calendar != nil {
		all = append(all,
			createEvent(d),
			updateEvent(d),
			deleteEvent(d),
		)
	}
	return append(all,
		getScores(d),
		getWeather(d),
		getHotTub(d),
		setHotTubTarget(d),
		getPool(d),
		setPoolTarget(d),
	)
}

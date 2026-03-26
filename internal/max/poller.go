package max

import "context"

// Poller runs MAX long polling for local development and test environments.
type Poller struct {
	Client     *Client
	Limit      int
	TimeoutSec int
	Types      []string
}

// Run keeps polling until the context is cancelled or one of the handlers
// returns an error.
func (p *Poller) Run(ctx context.Context, handler func(context.Context, Update) error) error {
	if p == nil || p.Client == nil {
		return nil
	}

	var marker *int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		page, err := p.Client.GetUpdates(ctx, GetUpdatesRequest{
			Limit:      p.Limit,
			TimeoutSec: p.TimeoutSec,
			Marker:     marker,
			Types:      p.Types,
		})
		if err != nil {
			return err
		}
		for _, update := range page.Updates {
			if err := handler(ctx, update); err != nil {
				return err
			}
		}
		if page.Marker != nil {
			next := *page.Marker
			marker = &next
		}
	}
}

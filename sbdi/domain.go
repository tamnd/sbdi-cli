package sbdi

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes sbdi as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/sbdi-cli/sbdi"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// sbdi:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone sbdi binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the sbdi driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "sbdi",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "sbdi",
			Short:  "Read Swedish Biodiversity Data Infrastructure occurrence records.",
			Long: `Read 175M+ biodiversity occurrence records from SBDI (biodiversitydata.se).

sbdi reads species observations, museum specimens, and field survey data from
Swedish institutions. No API key required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/sbdi-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// search: text search across occurrences
	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search SBDI occurrence records by taxon, keyword (--limit, --offset)",
		Args:    []kit.Arg{{Name: "query", Help: "taxon name or keyword"}}}, searchOccurrences)

	// recent: most recent occurrences (q=*, sorted by date)
	kit.Handle(app, kit.OpMeta{Name: "recent", Group: "read", List: true,
		Summary: "List recent SBDI occurrences (--limit)"}, recentOccurrences)
}

// newClient builds the client from the host-resolved config, so a host and the
// standalone binary pace and identify themselves the same way.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"          help:"taxon name or keyword"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag"         help:"result offset"`
	Client *Client `kit:"inject"`
}

type recentInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchOccurrences(ctx context.Context, in searchInput, emit func(*Occurrence) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	results, _, err := in.Client.SearchOccurrences(ctx, in.Query, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, o := range results {
		if err := emit(o); err != nil {
			return err
		}
	}
	return nil
}

func recentOccurrences(ctx context.Context, in recentInput, emit func(*Occurrence) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	results, _, err := in.Client.SearchOccurrences(ctx, "*", limit, 0)
	if err != nil {
		return mapErr(err)
	}
	for _, o := range results {
		if err := emit(o); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any accepted input into the canonical (type, id).
// Any non-empty string is treated as an occurrence ID.
func (Domain) Classify(input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty SBDI reference")
	}
	return "occurrence", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "occurrence":
		return "https://records.biodiversitydata.se/occurrences/" + id, nil
	default:
		return "", errs.Usage("sbdi has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}

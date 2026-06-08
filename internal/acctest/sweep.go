package acctest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/dovys/terraform-provider-unifi/internal/unifi"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/access"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/console"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/network"
	"github.com/dovys/terraform-provider-unifi/internal/unifi/protect"
)

// IsNotFound reports whether err is any of the client not-found sentinels. Each
// substruct client (Network/Protect/Access) and the console client exposes its
// own distinct ErrNotFound, so a destroy check must match them all — the same
// set the provider's remove-on-404 logic uses.
func IsNotFound(err error) bool {
	return errors.Is(err, unifi.ErrNotFound) ||
		errors.Is(err, network.ErrNotFound) ||
		errors.Is(err, protect.ErrNotFound) ||
		errors.Is(err, access.ErrNotFound) ||
		errors.Is(err, console.ErrNotFound)
}

// SweepItem is one object a sweeper considers: its id and natural-key name.
type SweepItem struct {
	ID   string
	Name string
}

// RegisterSweeper deletes every object of a type whose name carries the tfacc-
// prefix (so it never touches operator-made objects). Each resource contributes
// only its List/Delete closures.
func RegisterSweeper(typ string, list func(unifi.Client, context.Context) ([]SweepItem, error), del func(unifi.Client, context.Context, string) error) {
	resource.AddTestSweepers(typ, &resource.Sweeper{
		Name: typ,
		F: func(string) error {
			c, err := Client()
			if err != nil {
				return err
			}
			ctx := context.Background()
			items, err := list(c, ctx)
			if err != nil {
				return err
			}
			for _, it := range items {
				if !strings.HasPrefix(it.Name, NamePrefix) {
					continue
				}
				if err := del(c, ctx, it.ID); err != nil {
					return fmt.Errorf("sweep %s %q (%s): %w", typ, it.Name, it.ID, err)
				}
			}
			return nil
		},
	})
}

// CheckDestroyGone asserts every test-managed object of the type is gone. get
// must return a not-found error once the object is deleted (V2 collections with
// no Get can scan their List — see AbsentInList).
func CheckDestroyGone(typ string, get func(unifi.Client, context.Context, string) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c, err := Client()
		if err != nil {
			return err
		}
		ctx := context.Background()
		for _, rs := range s.RootModule().Resources {
			if rs.Type != typ {
				continue
			}
			switch err := get(c, ctx, rs.Primary.ID); {
			case err == nil:
				return fmt.Errorf("%s %s still exists after destroy", typ, rs.Primary.ID)
			case !IsNotFound(err):
				return fmt.Errorf("checking destroy of %s %s: %w", typ, rs.Primary.ID, err)
			}
		}
		return nil
	}
}

// AbsentInList turns a V2 collection's List into a Get-style existence check:
// nil if the id is present, ErrNotFound once it's gone.
func AbsentInList(ids func() ([]string, error), id string) error {
	got, err := ids()
	if err != nil {
		return err
	}
	for _, g := range got {
		if g == id {
			return nil
		}
	}
	return unifi.ErrNotFound
}

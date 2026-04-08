package manifest

import "fmt"

// ChangeType identifies the kind of manifest change detected.
type ChangeType string

const (
	ChangeAddTable     ChangeType = "ADD_TABLE"
	ChangeAddColumn    ChangeType = "ADD_COLUMN"
	ChangeDropColumn   ChangeType = "DROP_COLUMN" // warning only
	ChangeAddRoute     ChangeType = "ADD_ROUTE"
	ChangeUpdateRoute  ChangeType = "UPDATE_ROUTE"
	ChangeRemoveRoute  ChangeType = "REMOVE_ROUTE"
	ChangeAddScript    ChangeType = "ADD_SCRIPT"
	ChangeUpdateScript ChangeType = "UPDATE_SCRIPT"
	ChangeRemoveScript ChangeType = "REMOVE_SCRIPT"
	ChangeAddSeed      ChangeType = "ADD_SEED"
)

// Change describes a single detected difference between two manifests.
type Change struct {
	Type   ChangeType
	Table  string  // for schema changes
	Column *Column // for ADD_COLUMN / DROP_COLUMN
	Schema *Schema // for ADD_TABLE
	Route  *Route  // for route changes
	Script *Script // for script changes
	Seed   *Seed   // for seed changes
	Detail string  // human-readable description
}

// Diff compares old against new and returns a list of typed changes.
// Passing nil for old treats every item in new as an addition.
func Diff(old, new *Manifest) []Change {
	var changes []Change

	// Normalise: treat nil old as an empty manifest.
	var oldM Manifest
	if old != nil {
		oldM = *old
	}

	// ── Schema diff ──────────────────────────────────────────────────────────

	// Index old schemas by table name.
	oldSchemas := make(map[string]*Schema, len(oldM.Schemas))
	for i := range oldM.Schemas {
		s := &oldM.Schemas[i]
		oldSchemas[s.Table] = s
	}

	for i := range new.Schemas {
		ns := &new.Schemas[i]
		os, exists := oldSchemas[ns.Table]
		if !exists {
			// New table.
			schemaCopy := *ns
			changes = append(changes, Change{
				Type:   ChangeAddTable,
				Table:  ns.Table,
				Schema: &schemaCopy,
				Detail: fmt.Sprintf("new table %q", ns.Table),
			})
			continue
		}

		// Existing table — diff columns.
		oldCols := make(map[string]*Column, len(os.Columns))
		for j := range os.Columns {
			c := &os.Columns[j]
			oldCols[c.Name] = c
		}

		for j := range ns.Columns {
			nc := &ns.Columns[j]
			if _, exists := oldCols[nc.Name]; !exists {
				colCopy := *nc
				changes = append(changes, Change{
					Type:   ChangeAddColumn,
					Table:  ns.Table,
					Column: &colCopy,
					Detail: fmt.Sprintf("new column %q on table %q", nc.Name, ns.Table),
				})
			}
		}

		// Detect removed columns (warning only).
		newCols := make(map[string]struct{}, len(ns.Columns))
		for _, nc := range ns.Columns {
			newCols[nc.Name] = struct{}{}
		}
		for j := range os.Columns {
			oc := &os.Columns[j]
			if _, exists := newCols[oc.Name]; !exists {
				colCopy := *oc
				changes = append(changes, Change{
					Type:   ChangeDropColumn,
					Table:  ns.Table,
					Column: &colCopy,
					Detail: fmt.Sprintf("WARNING: column %q removed from table %q", oc.Name, ns.Table),
				})
			}
		}
	}

	// ── Route diff ───────────────────────────────────────────────────────────

	type routeKey struct{ Method, Path string }

	oldRoutes := make(map[routeKey]*Route, len(oldM.Routes))
	for i := range oldM.Routes {
		r := &oldM.Routes[i]
		oldRoutes[routeKey{r.Method, r.Path}] = r
	}

	newRoutes := make(map[routeKey]*Route, len(new.Routes))
	for i := range new.Routes {
		r := &new.Routes[i]
		newRoutes[routeKey{r.Method, r.Path}] = r
	}

	for k, nr := range newRoutes {
		or_, exists := oldRoutes[k]
		routeCopy := *nr
		if !exists {
			changes = append(changes, Change{
				Type:   ChangeAddRoute,
				Route:  &routeCopy,
				Detail: fmt.Sprintf("new route %s %s", nr.Method, nr.Path),
			})
		} else if or_.Script != nr.Script || or_.Description != nr.Description {
			changes = append(changes, Change{
				Type:   ChangeUpdateRoute,
				Route:  &routeCopy,
				Detail: fmt.Sprintf("updated route %s %s", nr.Method, nr.Path),
			})
		}
	}

	for k, or_ := range oldRoutes {
		if _, exists := newRoutes[k]; !exists {
			routeCopy := *or_
			changes = append(changes, Change{
				Type:   ChangeRemoveRoute,
				Route:  &routeCopy,
				Detail: fmt.Sprintf("removed route %s %s", or_.Method, or_.Path),
			})
		}
	}

	// ── Script diff ──────────────────────────────────────────────────────────

	oldScripts := make(map[string]*Script, len(oldM.Scripts))
	for i := range oldM.Scripts {
		s := &oldM.Scripts[i]
		oldScripts[s.Name] = s
	}

	newScripts := make(map[string]*Script, len(new.Scripts))
	for i := range new.Scripts {
		s := &new.Scripts[i]
		newScripts[s.Name] = s
	}

	for name, ns := range newScripts {
		os_, exists := oldScripts[name]
		scriptCopy := *ns
		if !exists {
			changes = append(changes, Change{
				Type:   ChangeAddScript,
				Script: &scriptCopy,
				Detail: fmt.Sprintf("new script %q", ns.Name),
			})
		} else if os_.Code != ns.Code {
			changes = append(changes, Change{
				Type:   ChangeUpdateScript,
				Script: &scriptCopy,
				Detail: fmt.Sprintf("updated script %q", ns.Name),
			})
		}
	}

	for name, os_ := range oldScripts {
		if _, exists := newScripts[name]; !exists {
			scriptCopy := *os_
			changes = append(changes, Change{
				Type:   ChangeRemoveScript,
				Script: &scriptCopy,
				Detail: fmt.Sprintf("removed script %q", os_.Name),
			})
		}
	}

	// ── Seed diff (additions only) ────────────────────────────────────────────

	oldSeeds := make(map[string]struct{}, len(oldM.Seeds))
	for _, s := range oldM.Seeds {
		oldSeeds[s.Table] = struct{}{}
	}

	for i := range new.Seeds {
		ns := &new.Seeds[i]
		if _, exists := oldSeeds[ns.Table]; !exists {
			seedCopy := *ns
			changes = append(changes, Change{
				Type:   ChangeAddSeed,
				Table:  ns.Table,
				Seed:   &seedCopy,
				Detail: fmt.Sprintf("new seed data for table %q", ns.Table),
			})
		}
	}

	return changes
}

package store

import (
	"context"
	"strings"
)

const gearCols = `g.id, g.user_id, g.name, g.category, g.weight_g, g.worn, g.consumable, g.note, g.inventory_ref, g.archived, g.created_at,
  (SELECT COUNT(1) FROM pack_items p WHERE p.gear_id = g.id)`

func scanGear(sc interface{ Scan(...any) error }) (*Gear, error) {
	var g Gear
	var worn, cons, arch int
	if err := sc.Scan(&g.ID, &g.UserID, &g.Name, &g.Category, &g.WeightG, &worn, &cons, &g.Note, &g.InventoryRef, &arch, &g.CreatedAt, &g.Packs); err != nil {
		return nil, notFound(err)
	}
	g.Worn, g.Consumable, g.Archived = worn == 1, cons == 1, arch == 1
	return &g, nil
}

// ListGear lists one person's gear by category and name.
func (s *Store) ListGear(ctx context.Context, uid int64, archived bool, q string) ([]*Gear, error) {
	query := `SELECT ` + gearCols + ` FROM gear g WHERE g.user_id = ? AND g.archived = ?`
	args := []any{uid, boolInt(archived)}
	if t := strings.TrimSpace(q); t != "" {
		query += ` AND (g.name LIKE ? OR g.note LIKE ?)`
		args = append(args, "%"+t+"%", "%"+t+"%")
	}
	rows, err := s.DB.QueryContext(ctx, query+` ORDER BY g.category, g.name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Gear{}
	for rows.Next() {
		g, err := scanGear(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGear is one piece of gear.
func (s *Store) GetGear(ctx context.Context, uid, id int64) (*Gear, error) {
	return scanGear(s.DB.QueryRowContext(ctx, `SELECT `+gearCols+` FROM gear g WHERE g.user_id = ? AND g.id = ?`, uid, id))
}

// GearByInventory finds the gear made from an item in Pocket Inventory.
func (s *Store) GearByInventory(ctx context.Context, uid int64, ref string) (int64, bool) {
	var id int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM gear WHERE user_id = ? AND inventory_ref = ?`, uid, ref).Scan(&id); err != nil {
		return 0, false
	}
	return id, true
}

// GearByName finds gear by its name (imports).
func (s *Store) GearByName(ctx context.Context, uid int64, name string) (int64, bool) {
	var id int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM gear WHERE user_id = ? AND name = ? COLLATE NOCASE`, uid, name).Scan(&id); err != nil {
		return 0, false
	}
	return id, true
}

// CreateGear adds gear.
func (s *Store) CreateGear(ctx context.Context, uid int64, g *Gear) error {
	g.UserID = uid
	res, err := s.DB.ExecContext(ctx, `INSERT INTO gear (user_id, name, category, weight_g, worn, consumable, note, inventory_ref, archived) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, g.Name, g.Category, g.WeightG, boolInt(g.Worn), boolInt(g.Consumable), g.Note, g.InventoryRef, boolInt(g.Archived))
	if err != nil {
		return err
	}
	g.ID, _ = res.LastInsertId()
	g.CreatedAt = Now()
	return nil
}

// UpdateGear saves gear.
func (s *Store) UpdateGear(ctx context.Context, g *Gear) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE gear SET name = ?, category = ?, weight_g = ?, worn = ?, consumable = ?, note = ?, inventory_ref = ?, archived = ? WHERE user_id = ? AND id = ?`,
		g.Name, g.Category, g.WeightG, boolInt(g.Worn), boolInt(g.Consumable), g.Note, g.InventoryRef, boolInt(g.Archived), g.UserID, g.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteGear removes gear (it leaves the packing lists; outings keep their copy).
func (s *Store) DeleteGear(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM gear WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- packing lists ----

// ListPacks lists one person's packing lists with their gear and weights.
func (s *Store) ListPacks(ctx context.Context, uid int64) ([]*Pack, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, name, activity, note, created_at FROM packs WHERE user_id = ? ORDER BY name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	var out []*Pack
	for rows.Next() {
		var p Pack
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Activity, &p.Note, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, &p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	gear, err := s.ListGear(ctx, uid, false, "")
	if err != nil {
		return nil, err
	}
	arch, err := s.ListGear(ctx, uid, true, "")
	if err != nil {
		return nil, err
	}
	byID := map[int64]*Gear{}
	for _, g := range append(gear, arch...) {
		byID[g.ID] = g
	}
	items, err := s.DB.QueryContext(ctx, `SELECT pack_id, gear_id, qty FROM pack_items WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	byPack := map[int64][]*PackItem{}
	for items.Next() {
		var pid int64
		var it PackItem
		if err := items.Scan(&pid, &it.GearID, &it.Qty); err != nil {
			items.Close()
			return nil, err
		}
		it.Gear = byID[it.GearID]
		byPack[pid] = append(byPack[pid], &it)
	}
	items.Close()
	if out == nil {
		out = []*Pack{}
	}
	for _, p := range out {
		p.Items = byPack[p.ID]
		if p.Items == nil {
			p.Items = []*PackItem{}
		}
		p.Count = len(p.Items)
		for _, it := range p.Items {
			if it.Gear != nil {
				p.Weight.Add(it.Gear.WeightG, it.Qty, it.Gear.Worn, it.Gear.Consumable)
			}
		}
	}
	return out, items.Err()
}

// GetPack is one packing list.
func (s *Store) GetPack(ctx context.Context, uid, id int64) (*Pack, error) {
	all, err := s.ListPacks(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

// CreatePack adds a packing list.
func (s *Store) CreatePack(ctx context.Context, uid int64, p *Pack) error {
	p.UserID = uid
	res, err := s.DB.ExecContext(ctx, `INSERT INTO packs (user_id, name, activity, note) VALUES (?, ?, ?, ?)`, uid, p.Name, p.Activity, p.Note)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// UpdatePack saves a packing list's name and note.
func (s *Store) UpdatePack(ctx context.Context, p *Pack) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE packs SET name = ?, activity = ?, note = ? WHERE user_id = ? AND id = ?`, p.Name, p.Activity, p.Note, p.UserID, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPackItems replaces what is on a packing list (gear of the person only).
func (s *Store) SetPackItems(ctx context.Context, uid, packID int64, items []PackItem) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM packs WHERE user_id = ? AND id = ?`, uid, packID).Scan(&n); err != nil || n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM pack_items WHERE user_id = ? AND pack_id = ?`, uid, packID); err != nil {
		return err
	}
	for _, it := range items {
		var mine int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM gear WHERE user_id = ? AND id = ?`, uid, it.GearID).Scan(&mine); err != nil || mine == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO pack_items (user_id, pack_id, gear_id, qty) VALUES (?, ?, ?, ?)`, uid, packID, it.GearID, max(it.Qty, 1)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePack removes a packing list (the gear stays).
func (s *Store) DeletePack(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM packs WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

package campaigns

import (
	"context"

	"github.com/keegancsmith/sqlf"
	"github.com/pkg/errors"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/db"
	"github.com/sourcegraph/sourcegraph/pkg/db/dbconn"
)

// dbCampaign describes a campaign.
type dbCampaign struct {
	ID          int64
	ProjectID   int64  // the project that defines the campaign
	Name        string // the name (case-preserving)
	Description *string
}

// errCampaignNotFound occurs when a database operation expects a specific changeset
// campaign to exist but it does not exist.
var errCampaignNotFound = errors.New("campaign not found")

type dbCampaigns struct{}

// Create creates a campaign. The campaign argument's (Campaign).ID
// field is ignored. The database ID of the new campaign is returned.
func (dbCampaigns) Create(ctx context.Context, campaign *dbCampaign) (*dbCampaign, error) {
	if mocks.campaigns.Create != nil {
		return mocks.campaigns.Create(campaign)
	}

	var id int64
	if err := dbconn.Global.QueryRowContext(ctx,
		`INSERT INTO changeset_campaigns(project_id, name, description) VALUES($1, $2, $3, $4) RETURNING id`,
		campaign.ProjectID, campaign.Name, campaign.Description,
	).Scan(&id); err != nil {
		return nil, err
	}
	created := *campaign
	created.ID = id
	return &created, nil
}

type dbCampaignUpdate struct {
	Name        *string
	Description *string
}

// Update updates a campaign given its ID.
func (s dbCampaigns) Update(ctx context.Context, id int64, update dbCampaignUpdate) (*dbCampaign, error) {
	if mocks.campaigns.Update != nil {
		return mocks.campaigns.Update(id, update)
	}

	var setFields []*sqlf.Query
	if update.Name != nil {
		setFields = append(setFields, sqlf.Sprintf("name=%s", *update.Name))
	}
	if update.Description != nil {
		// Treat empty string as meaning "set to null". Otherwise there is no way to express that
		// intent.
		var value *string
		if *update.Description != "" {
			value = update.Description
		}
		setFields = append(setFields, sqlf.Sprintf("description=%s", value))
	}

	if len(setFields) == 0 {
		return nil, nil
	}

	results, err := s.query(ctx, sqlf.Sprintf(`UPDATE campaigns SET %v WHERE id=%s RETURNING id, project_id, name, description`, sqlf.Join(setFields, ", "), id))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errCampaignNotFound
	}
	return results[0], nil
}

// GetByID retrieves the campaign (if any) given its ID.
//
// 🚨 SECURITY: The caller must ensure that the actor is permitted to view this campaign.
func (s dbCampaigns) GetByID(ctx context.Context, id int64) (*dbCampaign, error) {
	if mocks.campaigns.GetByID != nil {
		return mocks.campaigns.GetByID(id)
	}

	results, err := s.list(ctx, []*sqlf.Query{sqlf.Sprintf("id=%d", id)}, nil)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errCampaignNotFound
	}
	return results[0], nil
}

// dbCampaignsListOptions contains options for listing campaigns.
type dbCampaignsListOptions struct {
	Query     string // only list campaigns matching this query (case-insensitively)
	ProjectID int64  // only list campaigns defined in this project
	*db.LimitOffset
}

func (o dbCampaignsListOptions) sqlConditions() []*sqlf.Query {
	conds := []*sqlf.Query{sqlf.Sprintf("TRUE")}
	if o.Query != "" {
		conds = append(conds, sqlf.Sprintf("name LIKE %s", "%"+o.Query+"%"))
	}
	if o.ProjectID != 0 {
		conds = append(conds, sqlf.Sprintf("project_id=%d", o.ProjectID))
	}
	return conds
}

// List lists all campaigns that satisfy the options.
//
// 🚨 SECURITY: The caller must ensure that the actor is permitted to list with the specified
// options.
func (s dbCampaigns) List(ctx context.Context, opt dbCampaignsListOptions) ([]*dbCampaign, error) {
	if mocks.campaigns.List != nil {
		return mocks.campaigns.List(opt)
	}

	return s.list(ctx, opt.sqlConditions(), opt.LimitOffset)
}

func (s dbCampaigns) list(ctx context.Context, conds []*sqlf.Query, limitOffset *db.LimitOffset) ([]*dbCampaign, error) {
	q := sqlf.Sprintf(`
SELECT id, project_id, name, description FROM campaigns
WHERE (%s)
ORDER BY name ASC
%s`,
		sqlf.Join(conds, ") AND ("),
		limitOffset.SQL(),
	)
	return s.query(ctx, q)
}

func (dbCampaigns) query(ctx context.Context, query *sqlf.Query) ([]*dbCampaign, error) {
	rows, err := dbconn.Global.QueryContext(ctx, query.Query(sqlf.PostgresBindVar), query.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*dbCampaign
	for rows.Next() {
		var t dbCampaign
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Description); err != nil {
			return nil, err
		}
		results = append(results, &t)
	}
	return results, nil
}

// Count counts all campaigns that satisfy the options (ignoring limit and offset).
//
// 🚨 SECURITY: The caller must ensure that the actor is permitted to count the campaigns.
func (dbCampaigns) Count(ctx context.Context, opt dbCampaignsListOptions) (int, error) {
	if mocks.campaigns.Count != nil {
		return mocks.campaigns.Count(opt)
	}

	q := sqlf.Sprintf("SELECT COUNT(*) FROM campaigns WHERE (%s)", sqlf.Join(opt.sqlConditions(), ") AND ("))
	var count int
	if err := dbconn.Global.QueryRowContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Delete deletes a campaign given its ID.
//
// 🚨 SECURITY: The caller must ensure that the actor is permitted to delete the campaign.
func (s dbCampaigns) DeleteByID(ctx context.Context, id int64) error {
	if mocks.campaigns.DeleteByID != nil {
		return mocks.campaigns.DeleteByID(id)
	}
	return s.delete(ctx, sqlf.Sprintf("id=%d", id))
}

func (dbCampaigns) delete(ctx context.Context, cond *sqlf.Query) error {
	conds := []*sqlf.Query{cond, sqlf.Sprintf("TRUE")}
	q := sqlf.Sprintf("DELETE FROM campaigns WHERE (%s)", sqlf.Join(conds, ") AND ("))

	res, err := dbconn.Global.ExecContext(ctx, q.Query(sqlf.PostgresBindVar), q.Args()...)
	if err != nil {
		return err
	}
	nrows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if nrows == 0 {
		return errCampaignNotFound
	}
	return nil
}

// mockCampaigns mocks the campaigns-related DB operations.
type mockCampaigns struct {
	Create     func(*dbCampaign) (*dbCampaign, error)
	Update     func(int64, dbCampaignUpdate) (*dbCampaign, error)
	GetByID    func(int64) (*dbCampaign, error)
	List       func(dbCampaignsListOptions) ([]*dbCampaign, error)
	Count      func(dbCampaignsListOptions) (int, error)
	DeleteByID func(int64) error
}
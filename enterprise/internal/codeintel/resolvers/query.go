package resolvers

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel/lsifserver/client"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/lsif"
)

type lsifQueryResolver struct {
	repoID  api.RepoID
	commit  graphqlbackend.GitObjectID
	path    string
	uploads []*lsif.LSIFUpload
}

var _ graphqlbackend.LSIFQueryResolver = &lsifQueryResolver{}

// TODO - replace me
// func (r *lsifQueryResolver) Commit(ctx context.Context) (*graphqlbackend.GitCommitResolver, error) {
// 	return resolveCommit(ctx, r.repoID, r.uploads[0].Commit)
// }

func (r *lsifQueryResolver) Definitions(ctx context.Context, args *graphqlbackend.LSIFQueryPositionArgs) (graphqlbackend.LocationConnectionResolver, error) {
	// TODO - deduplicate
	// TODO - re-order
	// TODO - request concurrently

	var allLocations []*lsif.LSIFLocation
	for _, upload := range r.uploads {
		opts := &struct {
			RepoID    api.RepoID
			Commit    graphqlbackend.GitObjectID
			Path      string
			Line      int32
			Character int32
			UploadID  int64
		}{
			RepoID:    r.repoID,
			Commit:    r.commit,
			Path:      r.path,
			Line:      args.Line,
			Character: args.Character,
			UploadID:  upload.ID,
		}

		locations, _, err := client.DefaultClient.Definitions(ctx, opts)
		if err != nil {
			return nil, err
		}
		allLocations = append(allLocations, locations...)
	}

	return &locationConnectionResolver{
		locations: allLocations,
	}, nil
}

func (r *lsifQueryResolver) References(ctx context.Context, args *graphqlbackend.LSIFPagedQueryPositionArgs) (graphqlbackend.LocationConnectionResolver, error) {
	// Decode a map of upload ids to the next url that serves
	// the new page of results. This may not include an entry
	// for every upload if their result sets have already been
	// exhausted.
	nextURLs, err := readCursor(args.After)
	if err != nil {
		return nil, err
	}

	// We need to maintain a symmetric map for the next page
	// of results that we can encode into the endCursor of
	// this request.
	newCursors := map[int64]string{}

	var allLocations []*lsif.LSIFLocation
	for _, upload := range r.uploads {
		opts := &struct {
			RepoID    api.RepoID
			Commit    graphqlbackend.GitObjectID
			Path      string
			Line      int32
			Character int32
			UploadID  int64
			Limit     *int32
			Cursor    *string
		}{
			RepoID:    r.repoID,
			Commit:    r.commit,
			Path:      r.path,
			Line:      args.Line,
			Character: args.Character,
			UploadID:  upload.ID,
		}
		if args.First != nil {
			opts.Limit = args.First
		}
		if nextURL, ok := nextURLs[upload.ID]; ok {
			opts.Cursor = &nextURL
		}

		locations, nextURL, err := client.DefaultClient.References(ctx, opts)
		if err != nil {
			return nil, err
		}
		allLocations = append(allLocations, locations...)

		if nextURL != "" {
			newCursors[upload.ID] = nextURL
		}
	}

	endCursor, err := makeCursor(newCursors)
	if err != nil {
		return nil, err
	}

	return &locationConnectionResolver{
		locations: allLocations,
		endCursor: endCursor,
	}, nil
}

func (r *lsifQueryResolver) Hover(ctx context.Context, args *graphqlbackend.LSIFQueryPositionArgs) (graphqlbackend.HoverResolver, error) {
	// TODO - re-order
	// TODO - request concurrently

	for _, upload := range r.uploads {
		text, lspRange, err := client.DefaultClient.Hover(ctx, &struct {
			RepoID    api.RepoID
			Commit    graphqlbackend.GitObjectID
			Path      string
			Line      int32
			Character int32
			UploadID  int64
		}{
			RepoID:    r.repoID,
			Commit:    r.commit,
			Path:      r.path,
			Line:      args.Line,
			Character: args.Character,
			UploadID:  upload.ID,
		})
		if err != nil {
			return nil, err
		}

		if text != "" {
			return &hoverResolver{
				text:     text,
				lspRange: lspRange,
			}, nil
		}
	}

	return nil, nil
}

// readCursor decodes a cursor into a map from upload ids to URLs that
// serves the next page of results.
func readCursor(after *string) (map[int64]string, error) {
	if after == nil {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(*after)
	if err != nil {
		return nil, err
	}

	var cursors map[int64]string
	if err := json.Unmarshal(decoded, &cursors); err != nil {
		return nil, err
	}
	return cursors, nil
}

// makeCursor encodes a map from upload ids to URLs that serves the next
// page of results into a single string that can be sent back for use in
// cursor pagination.
func makeCursor(cursors map[int64]string) (string, error) {
	if len(cursors) == 0 {
		return "", nil
	}

	encoded, err := json.Marshal(cursors)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encoded), nil
}

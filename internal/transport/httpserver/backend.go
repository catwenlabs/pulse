package httpserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/opml"
	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/preview"
	"github.com/catwenlabs/pulse/internal/rule"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

type sourceRepository interface {
	Create(context.Context, source.Spec) (source.Source, error)
	List(context.Context) ([]source.Source, error)
	Get(context.Context, source.ID) (source.Source, error)
	Update(context.Context, source.ID, source.Spec) (source.Source, error)
	SetEnabled(context.Context, source.ID, bool) error
	Archive(context.Context, source.ID) error
	SetSecretRef(context.Context, source.ID, string) error
	Health(context.Context, source.ID) (source.Health, error)
}

type acquisitionQueue interface {
	Enqueue(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error)
}

type entryRepository interface {
	Get(context.Context, entry.ID) (entry.Entry, error)
	Delete(context.Context, entry.ID, bool) error
}

type sourceEntryRepository interface {
	SearchSourceEntries(context.Context, entry.Query) ([]story.SourceEntry, error)
	SearchSourceEntryPage(context.Context, entry.Query) (story.SourceEntryPage, error)
}

type storyRepository interface {
	Search(context.Context, story.Query) ([]story.Story, error)
	SearchPage(context.Context, story.Query) (story.Page, error)
	Get(context.Context, story.ID) (story.Story, error)
	Update(context.Context, story.ID, story.Patch) (story.Story, error)
	SetRepresentative(context.Context, story.ID, entry.ID) (story.Story, error)
	MarkRead(context.Context, string) (int64, error)
	MergeManual(context.Context, story.ID, story.ID, story.MergeOptions) error
	Split(context.Context, story.ID, entry.ID, story.SplitOptions) (story.ID, error)
	AddTag(context.Context, story.ID, string) (entry.Tag, error)
	RemoveTag(context.Context, story.ID, string) error
}

// storyReclusterer runs an on-demand Story aggregation pass. It is optional: when
// nil (no aggregation processor wired in), Recluster returns ErrReclusterUnavailable.
type storyReclusterer interface {
	RunOnce(context.Context, int) (int, error)
}

type opmlRepository interface {
	Import(context.Context, []opml.Subscription) (opml.ImportResult, error)
	List(context.Context) ([]opml.Subscription, error)
}

type sourcePreviewer interface {
	Run(context.Context, source.Spec) (preview.Result, error)
}

type organizationRepository interface {
	CreateFolder(context.Context, string) (organization.Folder, error)
	ListFolders(context.Context) ([]organization.Folder, error)
	DeleteFolder(context.Context, string) error
	AddSourceToFolder(context.Context, string, source.ID) error
	RemoveSourceFromFolder(context.Context, string, source.ID) error
	CreateView(context.Context, organization.View) (organization.View, error)
	UpdateView(context.Context, organization.View) (organization.View, error)
	ListViews(context.Context) ([]organization.View, error)
	DeleteView(context.Context, string) error
}

type ruleRepository interface {
	Create(context.Context, rule.Rule) (rule.Rule, error)
	List(context.Context) ([]rule.Rule, error)
	Get(context.Context, string) (rule.Rule, error)
	Update(context.Context, rule.Rule) (rule.Rule, error)
	Delete(context.Context, string) error
	Preview(context.Context, string) (rule.PreviewResult, error)
	Replay(context.Context, string, bool) (rule.ReplayResult, error)
}

type backend struct {
	sources      sourceRepository
	acquisitions acquisitionQueue
	entries      entryRepository
	opml         opmlRepository
	previewer    sourcePreviewer
	organization organizationRepository
	rules        ruleRepository
	stories      storyRepository
	reclusterer  storyReclusterer
}

func NewBackend(
	sources sourceRepository,
	acquisitions acquisitionQueue,
	entries entryRepository,
	opmlRepository opmlRepository,
	previewer sourcePreviewer,
	organizationStore organizationRepository,
	storyStore storyRepository,
	reclusterer storyReclusterer,
	ruleStores ...ruleRepository,
) Backend {
	service := &backend{
		sources:      sources,
		acquisitions: acquisitions,
		entries:      entries,
		opml:         opmlRepository,
		previewer:    previewer,
		organization: organizationStore,
		stories:      storyStore,
		reclusterer:  reclusterer,
	}
	if len(ruleStores) > 0 {
		service.rules = ruleStores[0]
	}
	return service
}

func (service *backend) ListStories(ctx context.Context, query story.Query) ([]story.Story, error) {
	return service.stories.Search(ctx, query)
}

func (service *backend) ListStoryPage(ctx context.Context, query story.Query) (story.Page, error) {
	return service.stories.SearchPage(ctx, query)
}

func (service *backend) GetStory(ctx context.Context, id story.ID) (story.Story, error) {
	return service.stories.Get(ctx, id)
}

func (service *backend) UpdateStory(
	ctx context.Context,
	id story.ID,
	patch story.Patch,
) (story.Story, error) {
	return service.stories.Update(ctx, id, patch)
}

func (service *backend) SetStoryRepresentative(ctx context.Context, storyID story.ID, entryID entry.ID) (story.Story, error) {
	return service.stories.SetRepresentative(ctx, storyID, entryID)
}

func (service *backend) MarkStoriesRead(ctx context.Context, sourceID string) (int64, error) {
	return service.stories.MarkRead(ctx, sourceID)
}

func (service *backend) MergeStories(
	ctx context.Context,
	from story.ID,
	into story.ID,
	options story.MergeOptions,
) (story.Story, error) {
	if err := service.stories.MergeManual(ctx, from, into, options); err != nil {
		return story.Story{}, err
	}
	return service.stories.Get(ctx, into)
}

func (service *backend) SplitStory(
	ctx context.Context,
	storyID story.ID,
	entryID entry.ID,
	options story.SplitOptions,
) (story.Story, error) {
	newID, err := service.stories.Split(ctx, storyID, entryID, options)
	if err != nil {
		return story.Story{}, err
	}
	return service.stories.Get(ctx, newID)
}

// Recluster drains pending Story aggregation on demand, re-evaluating single-Entry
// Stories (and embedding backfill) instead of waiting for the background tick. It is
// bounded so a large backlog cannot keep an HTTP request open indefinitely.
func (service *backend) Recluster(ctx context.Context) (int, error) {
	if service.reclusterer == nil {
		return 0, story.ErrReclusterUnavailable
	}
	const batchSize = 50
	const maxPasses = 200
	total := 0
	for pass := 0; pass < maxPasses; pass++ {
		processed, err := service.reclusterer.RunOnce(ctx, batchSize)
		if err != nil {
			slog.Error("Story recluster pass failed", "pass", pass+1, "processed_total", total, "error", err)
			return total, err
		}
		total += processed
		slog.Info("Story recluster pass", "pass", pass+1, "processed_this_pass", processed, "processed_total", total)
		if processed < batchSize {
			break
		}
	}
	return total, nil
}

func (service *backend) CreateRule(ctx context.Context, definition rule.Rule) (rule.Rule, error) {
	return service.rules.Create(ctx, definition)
}
func (service *backend) ListRules(ctx context.Context) ([]rule.Rule, error) {
	return service.rules.List(ctx)
}
func (service *backend) GetRule(ctx context.Context, id string) (rule.Rule, error) {
	return service.rules.Get(ctx, id)
}
func (service *backend) UpdateRule(ctx context.Context, definition rule.Rule) (rule.Rule, error) {
	return service.rules.Update(ctx, definition)
}
func (service *backend) DeleteRule(ctx context.Context, id string) error {
	return service.rules.Delete(ctx, id)
}
func (service *backend) PreviewRule(ctx context.Context, id string) (rule.PreviewResult, error) {
	return service.rules.Preview(ctx, id)
}
func (service *backend) ReplayRule(ctx context.Context, id string, effects bool) (rule.ReplayResult, error) {
	return service.rules.Replay(ctx, id, effects)
}

func (service *backend) CreateFolder(ctx context.Context, name string) (organization.Folder, error) {
	return service.organization.CreateFolder(ctx, name)
}
func (service *backend) ListFolders(ctx context.Context) ([]organization.Folder, error) {
	return service.organization.ListFolders(ctx)
}
func (service *backend) DeleteFolder(ctx context.Context, id string) error {
	return service.organization.DeleteFolder(ctx, id)
}
func (service *backend) AddSourceToFolder(ctx context.Context, folderID string, sourceID source.ID) error {
	return service.organization.AddSourceToFolder(ctx, folderID, sourceID)
}
func (service *backend) RemoveSourceFromFolder(ctx context.Context, folderID string, sourceID source.ID) error {
	return service.organization.RemoveSourceFromFolder(ctx, folderID, sourceID)
}
func (service *backend) CreateView(ctx context.Context, view organization.View) (organization.View, error) {
	return service.organization.CreateView(ctx, view)
}
func (service *backend) UpdateView(ctx context.Context, view organization.View) (organization.View, error) {
	return service.organization.UpdateView(ctx, view)
}
func (service *backend) ListViews(ctx context.Context) ([]organization.View, error) {
	return service.organization.ListViews(ctx)
}
func (service *backend) DeleteView(ctx context.Context, id string) error {
	return service.organization.DeleteView(ctx, id)
}

func (service *backend) PreviewSource(ctx context.Context, spec source.Spec) (preview.Result, error) {
	return service.previewer.Run(ctx, spec)
}

func (service *backend) ImportOPML(
	ctx context.Context,
	subscriptions []opml.Subscription,
) (opml.ImportResult, error) {
	return service.opml.Import(ctx, subscriptions)
}

func (service *backend) ExportOPML(ctx context.Context) ([]opml.Subscription, error) {
	return service.opml.List(ctx)
}

func (service *backend) CreateSource(ctx context.Context, spec source.Spec) (source.Source, error) {
	return service.sources.Create(ctx, spec)
}

func (service *backend) ListSources(ctx context.Context) ([]source.Source, error) {
	return service.sources.List(ctx)
}

func (service *backend) GetSource(ctx context.Context, id source.ID) (source.Source, error) {
	item, err := service.sources.Get(ctx, id)
	if err != nil {
		return source.Source{}, err
	}
	if item.ArchivedAt != nil {
		return source.Source{}, fmt.Errorf("%w: %s", source.ErrNotFound, id)
	}
	return item, nil
}

func (service *backend) SetSourceEnabled(
	ctx context.Context,
	id source.ID,
	enabled bool,
) (source.Source, error) {
	if err := service.sources.SetEnabled(ctx, id, enabled); err != nil {
		return source.Source{}, err
	}
	return service.sources.Get(ctx, id)
}

func (service *backend) UpdateSource(
	ctx context.Context,
	id source.ID,
	name string,
	locator string,
) (source.Source, error) {
	current, err := service.GetSource(ctx, id)
	if err != nil {
		return source.Source{}, err
	}
	return service.sources.Update(ctx, id, source.Spec{
		Name:      name,
		Kind:      current.Kind,
		Locator:   locator,
		Config:    current.Config,
		SecretRef: current.SecretRef,
	})
}

func (service *backend) ArchiveSource(ctx context.Context, id source.ID) error {
	return service.sources.Archive(ctx, id)
}

func (service *backend) SetSourceSecret(
	ctx context.Context,
	id source.ID,
	secret string,
) error {
	return service.sources.SetSecretRef(ctx, id, secret)
}

func (service *backend) GetSourceHealth(ctx context.Context, id source.ID) (source.Health, error) {
	if _, err := service.GetSource(ctx, id); err != nil {
		return source.Health{}, err
	}
	return service.sources.Health(ctx, id)
}

func (service *backend) Enqueue(
	ctx context.Context,
	request ingestion.EnqueueRequest,
) (ingestion.Acquisition, error) {
	return service.acquisitions.Enqueue(ctx, request)
}

func (service *backend) ListSourceEntries(
	ctx context.Context,
	sourceID source.ID,
	query entry.Query,
) ([]story.SourceEntry, error) {
	query.SourceID = sourceID
	repository, ok := service.entries.(sourceEntryRepository)
	if !ok {
		return nil, fmt.Errorf("source Entry browsing is unavailable")
	}
	return repository.SearchSourceEntries(ctx, query)
}

func (service *backend) ListSourceEntryPage(
	ctx context.Context,
	sourceID source.ID,
	query entry.Query,
) (story.SourceEntryPage, error) {
	query.SourceID = sourceID
	repository, ok := service.entries.(sourceEntryRepository)
	if !ok {
		return story.SourceEntryPage{}, fmt.Errorf("source Entry browsing is unavailable")
	}
	return repository.SearchSourceEntryPage(ctx, query)
}

func (service *backend) GetEntry(ctx context.Context, id entry.ID) (entry.Entry, error) {
	return service.entries.Get(ctx, id)
}

func (service *backend) DeleteEntry(ctx context.Context, id entry.ID, confirmed bool) error {
	return service.entries.Delete(ctx, id, confirmed)
}

func (service *backend) AddStoryTag(ctx context.Context, id story.ID, name string) (entry.Tag, error) {
	return service.stories.AddTag(ctx, id, name)
}

func (service *backend) RemoveStoryTag(ctx context.Context, id story.ID, tagID string) error {
	return service.stories.RemoveTag(ctx, id, tagID)
}

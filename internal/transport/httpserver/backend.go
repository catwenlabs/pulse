package httpserver

import (
	"context"
	"fmt"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/opml"
	"github.com/wenpengfei/pulse/internal/organization"
	"github.com/wenpengfei/pulse/internal/preview"
	"github.com/wenpengfei/pulse/internal/rule"
	"github.com/wenpengfei/pulse/internal/source"
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
	List(context.Context, int) ([]entry.Entry, error)
	Search(context.Context, entry.Query) ([]entry.Entry, error)
	Get(context.Context, entry.ID) (entry.Entry, error)
	Update(context.Context, entry.ID, entry.Patch) (entry.Entry, error)
	MarkRead(context.Context, source.ID) (int64, error)
	AddTag(context.Context, entry.ID, string) (entry.Tag, error)
	RemoveTag(context.Context, entry.ID, string) error
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
}

func NewBackend(
	sources sourceRepository,
	acquisitions acquisitionQueue,
	entries entryRepository,
	opmlRepository opmlRepository,
	previewer sourcePreviewer,
	organizationStore organizationRepository,
	ruleStores ...ruleRepository,
) Backend {
	service := &backend{
		sources:      sources,
		acquisitions: acquisitions,
		entries:      entries,
		opml:         opmlRepository,
		previewer:    previewer,
		organization: organizationStore,
	}
	if len(ruleStores) > 0 {
		service.rules = ruleStores[0]
	}
	return service
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

func (service *backend) ListEntries(ctx context.Context, limit int) ([]entry.Entry, error) {
	return service.entries.List(ctx, limit)
}

func (service *backend) SearchEntries(ctx context.Context, query entry.Query) ([]entry.Entry, error) {
	return service.entries.Search(ctx, query)
}

func (service *backend) GetEntry(ctx context.Context, id entry.ID) (entry.Entry, error) {
	return service.entries.Get(ctx, id)
}

func (service *backend) UpdateEntry(ctx context.Context, id entry.ID, patch entry.Patch) (entry.Entry, error) {
	return service.entries.Update(ctx, id, patch)
}

func (service *backend) MarkEntriesRead(ctx context.Context, sourceID source.ID) (int64, error) {
	return service.entries.MarkRead(ctx, sourceID)
}

func (service *backend) AddEntryTag(ctx context.Context, id entry.ID, name string) (entry.Tag, error) {
	return service.entries.AddTag(ctx, id, name)
}

func (service *backend) RemoveEntryTag(ctx context.Context, id entry.ID, tagID string) error {
	return service.entries.RemoveTag(ctx, id, tagID)
}

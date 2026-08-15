package cnpclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/izzyreal/ciwi/pkg/cnp"
	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

const (
	ALPN                        = cnp.ALPN
	projectIconsBatchCapability = "project_icons_batch"
)

type Error struct {
	Code    cnpv1.StatusCode
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Client struct {
	session      cnp.Session
	welcome      *cnpv1.Welcome
	projectIcons *ProjectIconCache
}

func Dial(ctx context.Context, address, clientName, clientVersion string) (*Client, error) {
	return DialWithProjectIconCache(ctx, address, clientName, clientVersion, nil)
}

func DialWithProjectIconCache(ctx context.Context, address, clientName, clientVersion string, icons *ProjectIconCache) (*Client, error) {
	target, err := ParseTarget(address)
	if err != nil {
		return nil, err
	}
	var session cnp.Session
	switch target.Transport {
	case TransportQUIC:
		session, err = dialQUIC(ctx, target.Address)
	case TransportTCP:
		session, err = dialTCP(ctx, target.Address)
	default:
		err = fmt.Errorf("unsupported native transport %q", target.Transport)
	}
	if err != nil {
		return nil, fmt.Errorf("dial ciwi native endpoint: %w", err)
	}
	return newClient(ctx, session, clientName, clientVersion, icons)
}

func newClient(ctx context.Context, session cnp.Session, clientName, clientVersion string, icons *ProjectIconCache) (*Client, error) {
	if icons == nil {
		icons = NewProjectIconCache()
	}
	client := &Client{session: session, projectIcons: icons}
	if err := client.hello(ctx, clientName, clientVersion); err != nil {
		_ = session.CloseWithError(fmt.Errorf("hello failed: %w", err))
		return nil, err
	}
	return client, nil
}

func (c *Client) Welcome() *cnpv1.Welcome { return c.welcome }

func (c *Client) serverInstallationID() string {
	if c == nil || c.welcome == nil {
		return ""
	}
	if installationID := strings.TrimSpace(c.welcome.ServerInstallationId); installationID != "" {
		return installationID
	}
	return strings.TrimSpace(c.welcome.ServerInstanceId)
}

func (c *Client) hasCapability(capability string) bool {
	return c != nil && c.welcome != nil && slices.Contains(c.welcome.Capabilities, capability)
}

func (c *Client) Close() error {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.CloseWithError(nil)
}

func (c *Client) hello(ctx context.Context, clientName, clientVersion string) error {
	stream, err := c.session.OpenStream(ctx)
	if err != nil {
		return contextualIOError(ctx, "open hello stream", err)
	}
	defer stream.Close()
	stopCancellation := interruptStreamOnCancel(ctx, stream)
	defer stopCancellation()
	message := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Hello{Hello: &cnpv1.Hello{
		ClientName: clientName, ClientVersion: clientVersion,
		Capabilities: []string{"protobuf", "invalidation_stream", "job_output_stream", "job_log_v1"},
	}}}
	if err := cnp.Write(stream, message); err != nil {
		stream.CancelRead()
		return contextualIOError(ctx, "write hello", err)
	}
	_ = stream.Close()
	var response cnpv1.ServerMessage
	if err := cnp.NewReader(stream).Read(&response); err != nil {
		return contextualIOError(ctx, "read welcome", err)
	}
	welcome := response.GetWelcome()
	if welcome == nil {
		return fmt.Errorf("native endpoint did not return welcome")
	}
	if !stopCancellation() {
		return fmt.Errorf("complete hello: %w", ctx.Err())
	}
	c.welcome = welcome
	return nil
}

func (c *Client) GetServerInfo(ctx context.Context) (*cnpv1.ServerInfo, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetServerInfo{GetServerInfo: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetServerInfo(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetCommandReceiptStatus(ctx context.Context, key string) (*cnpv1.CommandReceiptStatus, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetCommandReceiptStatus{
		GetCommandReceiptStatus: &cnpv1.CommandReceiptStatusRequest{Key: key},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetCommandReceiptStatus(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetServerUpdateStatus(ctx context.Context) (*cnpv1.ServerUpdateStatus, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetServerUpdateStatus{GetServerUpdateStatus: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetServerUpdateStatus(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) CheckServerUpdates(ctx context.Context) (*cnpv1.ServerUpdateCheckResult, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_CheckServerUpdates{CheckServerUpdates: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetServerUpdateCheck(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ListServerUpdateVersions(ctx context.Context) (*cnpv1.ServerUpdateVersions, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ListServerUpdateVersions{ListServerUpdateVersions: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetServerUpdateVersions(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ServerUpdateAction(ctx context.Context, action, targetVersion string) (*cnpv1.ServerUpdateActionResult, error) {
	return c.ServerUpdateActionWithKey(ctx, action, targetVersion, "")
}

func (c *Client) ServerUpdateActionWithKey(ctx context.Context, action, targetVersion, idempotencyKey string) (*cnpv1.ServerUpdateActionResult, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ServerUpdateAction{ServerUpdateAction: &cnpv1.ServerUpdateActionRequest{
		Action: action, TargetVersion: targetVersion,
	}}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetServerUpdateAction(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ListProjects(ctx context.Context) (*cnpv1.ProjectList, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ListProjects{ListProjects: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectList(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetFrontPageView(ctx context.Context) (*cnpv1.FrontPageView, error) {
	result, err := c.getFrontPageView(ctx, nil)
	if err != nil {
		return nil, err
	}
	missing := make([]int64, 0, len(result.Projects))
	iconHydrationSucceeded := true
	installationID := c.serverInstallationID()
	for _, project := range result.Projects {
		if project == nil {
			continue
		}
		icon, ok := c.projectIcons.get(installationID, project.Id)
		if !ok || icon.loadedCommit != project.LoadedCommit {
			missing = append(missing, project.Id)
		}
	}
	if len(missing) > 0 {
		iconHydrationSucceeded = false
		if c.hasCapability(projectIconsBatchCapability) {
			if icons, iconErr := c.GetProjectIcons(ctx, missing); iconErr == nil {
				iconHydrationSucceeded = true
				commits := make(map[int64]string, len(result.Projects))
				for _, project := range result.Projects {
					if project != nil {
						commits[project.Id] = project.LoadedCommit
					}
				}
				for _, icon := range icons.Icons {
					if icon == nil || icon.ProjectId <= 0 || len(icon.Data) == 0 {
						continue
					}
					c.projectIcons.put(installationID, icon.ProjectId, projectIcon{
						contentType: icon.ContentType, data: icon.Data, loadedCommit: commits[icon.ProjectId],
					})
				}
			}
		} else if hydrated, hydrateErr := c.getFrontPageView(ctx, missing); hydrateErr == nil {
			// Legacy servers can only return icons as part of another complete
			// front-page response. Treat that optional response as best-effort.
			result = hydrated
			iconHydrationSucceeded = true
		}
	}
	c.decorateProjectIcons(result.Projects, iconHydrationSucceeded)
	return result, nil
}

func (c *Client) GetProjectIcons(ctx context.Context, projectIDs []int64) (*cnpv1.ProjectIconList, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetProjectIcons{
		GetProjectIcons: &cnpv1.GetProjectIconsRequest{ProjectIds: append([]int64(nil), projectIDs...)},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectIcons(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) getFrontPageView(ctx context.Context, projectIconIDs []int64) (*cnpv1.FrontPageView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetFrontPageView{
		GetFrontPageView: &cnpv1.GetFrontPageViewRequest{IncludeProjectIconIds: projectIconIDs},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetFrontPageView(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetProjectDetails(ctx context.Context, projectID int64) (*cnpv1.ProjectDetailsView, error) {
	installationID := c.serverInstallationID()
	cachedIcon, hasCachedIcon := c.projectIcons.get(installationID, projectID)
	// A front-page response from a server predating summary icons leaves a
	// negative cache entry. Still ask the project-details operation for its
	// established top-level icon so details-page logos remain compatible.
	result, err := c.getProjectDetails(ctx, projectID, !hasCachedIcon || len(cachedIcon.data) == 0)
	if err != nil {
		return nil, err
	}
	loadedCommit := ""
	if result.Project != nil {
		loadedCommit = result.Project.LoadedCommit
		if len(result.ProjectIcon) == 0 && len(result.Project.ProjectIcon) > 0 {
			result.ProjectIcon = append([]byte(nil), result.Project.ProjectIcon...)
			result.ProjectIconContentType = result.Project.ProjectIconContentType
		}
	}
	if len(result.ProjectIcon) == 0 && hasCachedIcon && cachedIcon.loadedCommit != loadedCommit {
		result, err = c.getProjectDetails(ctx, projectID, true)
		if err != nil {
			return nil, err
		}
		if result.Project != nil {
			loadedCommit = result.Project.LoadedCommit
		}
	}
	if len(result.ProjectIcon) > 0 {
		cachedIcon = projectIcon{
			contentType:  result.ProjectIconContentType,
			data:         append([]byte(nil), result.ProjectIcon...),
			loadedCommit: loadedCommit,
		}
		hasCachedIcon = true
		c.projectIcons.put(installationID, projectID, cachedIcon)
	} else if hasCachedIcon && cachedIcon.loadedCommit == loadedCommit {
		result.ProjectIcon = append([]byte(nil), cachedIcon.data...)
		result.ProjectIconContentType = cachedIcon.contentType
	} else {
		cachedIcon = projectIcon{loadedCommit: loadedCommit}
		c.projectIcons.put(installationID, projectID, cachedIcon)
	}
	if result.Project != nil {
		result.Project.ProjectIcon = append([]byte(nil), result.ProjectIcon...)
		result.Project.ProjectIconContentType = result.ProjectIconContentType
	}
	return result, nil
}

func (c *Client) decorateProjectIcons(projects []*cnpv1.ProjectSummary, cacheMissing bool) {
	installationID := c.serverInstallationID()
	for _, project := range projects {
		if project == nil {
			continue
		}
		if len(project.ProjectIcon) > 0 {
			c.projectIcons.put(installationID, project.Id, projectIcon{
				contentType: project.ProjectIconContentType,
				data:        append([]byte(nil), project.ProjectIcon...), loadedCommit: project.LoadedCommit,
			})
			continue
		}
		if cached, ok := c.projectIcons.get(installationID, project.Id); ok && cached.loadedCommit == project.LoadedCommit {
			project.ProjectIcon = append([]byte(nil), cached.data...)
			project.ProjectIconContentType = cached.contentType
			continue
		}
		if cacheMissing {
			c.projectIcons.put(installationID, project.Id, projectIcon{loadedCommit: project.LoadedCommit})
		}
	}
}

func (c *Client) getProjectDetails(ctx context.Context, projectID int64, includeProjectIcon bool) (*cnpv1.ProjectDetailsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetProjectDetails{
		GetProjectDetails: &cnpv1.GetProjectDetailsRequest{ProjectId: projectID, IncludeProjectIcon: includeProjectIcon},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectDetails(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ProjectAction(ctx context.Context, projectID int64, action, idempotencyKey string) (*cnpv1.ProjectActionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ProjectAction{
		ProjectAction: &cnpv1.ProjectActionRequest{ProjectId: projectID, Action: action},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetProjectAction(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ImportProject(ctx context.Context, request *cnpv1.ImportProjectRequest, idempotencyKey string) (*cnpv1.ImportProjectResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ImportProject{ImportProject: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetImportProject(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetManagedYAML(ctx context.Context, projectID int64) (*cnpv1.ManagedYAMLDefinition, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetManagedYaml{
		GetManagedYaml: &cnpv1.GetManagedYAMLRequest{ProjectId: projectID},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetManagedYaml(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ValidateManagedYAML(ctx context.Context, request *cnpv1.ManagedYAMLRequest) (*cnpv1.ManagedYAMLDefinition, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ValidateManagedYaml{ValidateManagedYaml: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetManagedYaml(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) SaveManagedYAML(ctx context.Context, request *cnpv1.ManagedYAMLRequest, idempotencyKey string) (*cnpv1.ManagedYAMLDefinition, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_SaveManagedYaml{SaveManagedYaml: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetManagedYaml(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ListVaultConnections(ctx context.Context) (*cnpv1.VaultConnectionList, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ListVaultConnections{ListVaultConnections: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetVaultConnectionList(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) UpsertVaultConnection(ctx context.Context, request *cnpv1.UpsertVaultConnectionRequest, idempotencyKey string) (*cnpv1.VaultConnection, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_UpsertVaultConnection{UpsertVaultConnection: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetVaultConnection(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) TestVaultConnection(ctx context.Context, id int64) (*cnpv1.TestVaultConnectionResult, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_TestVaultConnection{TestVaultConnection: &cnpv1.TestVaultConnectionRequest{Id: id}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetTestVaultConnection(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) DeleteVaultConnection(ctx context.Context, id int64, idempotencyKey string) (*cnpv1.DeleteVaultConnectionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_DeleteVaultConnection{DeleteVaultConnection: &cnpv1.VaultConnectionIDRequest{Id: id}}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetDeleteVaultConnection(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetRunOptions(ctx context.Context, request *cnpv1.GetRunOptionsRequest) (*cnpv1.RunOptionsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetRunOptions{GetRunOptions: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetRunOptions(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetAgentsView(ctx context.Context) (*cnpv1.AgentsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetAgentsView{GetAgentsView: &cnpv1.Empty{}}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetAgentsView(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetAgentDetails(ctx context.Context, agentID string) (*cnpv1.AgentDetailsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetAgentDetails{
		GetAgentDetails: &cnpv1.GetAgentDetailsRequest{AgentId: agentID},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetAgentDetails(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) AgentAction(ctx context.Context, request *cnpv1.AgentActionRequest, idempotencyKey string) (*cnpv1.AgentActionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_AgentAction{AgentAction: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetAgentAction(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RunAgentScript(ctx context.Context, request *cnpv1.RunAgentScriptRequest, idempotencyKey string) (*cnpv1.RunAgentScriptResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RunAgentScript{RunAgentScript: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRunAgentScript(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetJobDetails(ctx context.Context, jobExecutionID string) (*cnpv1.JobDetailsView, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetJobDetails{
		GetJobDetails: &cnpv1.GetJobDetailsRequest{JobExecutionId: jobExecutionID, IncludeProjectIcon: true},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetJobDetails(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetJobLogDescriptor(ctx context.Context, jobExecutionID string) (*cnpv1.JobLogDescriptor, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetJobLogDescriptor{
		GetJobLogDescriptor: &cnpv1.JobLogDescriptorRequest{JobExecutionId: jobExecutionID},
	}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetJobLogDescriptor(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) GetJobLogPage(ctx context.Context, request *cnpv1.JobLogPageRequest) (*cnpv1.JobLogPage, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_GetJobLogPage{GetJobLogPage: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetJobLogPage(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) SearchJobLog(ctx context.Context, request *cnpv1.JobLogSearchRequest) (*cnpv1.JobLogSearchResult, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_SearchJobLog{SearchJobLog: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetJobLogSearch(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) DownloadArtifactChunk(ctx context.Context, request *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error) {
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_DownloadArtifact{DownloadArtifact: request}}, "")
	if err != nil {
		return nil, err
	}
	if result := response.GetArtifactDownload(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RunPipeline(ctx context.Context, request *cnpv1.RunPipelineRequest, idempotencyKey string) (*cnpv1.RunPipelineResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RunPipeline{RunPipeline: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRunPipeline(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RunPipelineChain(ctx context.Context, request *cnpv1.RunPipelineChainRequest, idempotencyKey string) (*cnpv1.RunPipelineChainResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RunPipelineChain{RunPipelineChain: request}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRunPipelineChain(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) ClearExecutionQueue(ctx context.Context, idempotencyKey string) (*cnpv1.ClearExecutionQueueResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_ClearExecutionQueue{
		ClearExecutionQueue: &cnpv1.ClearExecutionQueueRequest{},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetClearExecutionQueue(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) FlushExecutionHistory(ctx context.Context, request *cnpv1.FlushExecutionHistoryRequest, idempotencyKey string) (*cnpv1.FlushExecutionHistoryResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_FlushExecutionHistory{
		FlushExecutionHistory: request,
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetFlushExecutionHistory(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RemoveQueuedExecution(ctx context.Context, jobExecutionID, idempotencyKey string) (*cnpv1.RemoveQueuedExecutionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RemoveQueuedExecution{
		RemoveQueuedExecution: &cnpv1.ControlExecutionRequest{JobExecutionId: jobExecutionID},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRemoveQueuedExecution(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) CancelExecution(ctx context.Context, jobExecutionID, idempotencyKey string) (*cnpv1.CancelExecutionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_CancelExecution{
		CancelExecution: &cnpv1.ControlExecutionRequest{JobExecutionId: jobExecutionID},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetCancelExecution(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) RerunExecution(ctx context.Context, jobExecutionID, idempotencyKey string) (*cnpv1.RerunExecutionResult, error) {
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	response, err := c.call(ctx, &cnpv1.Request{Operation: &cnpv1.Request_RerunExecution{
		RerunExecution: &cnpv1.ControlExecutionRequest{JobExecutionId: jobExecutionID},
	}}, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if result := response.GetRerunExecution(); result != nil {
		return result, nil
	}
	return nil, unexpectedResult(response)
}

func (c *Client) WatchChanges(ctx context.Context) (<-chan *cnpv1.ChangeEvent, <-chan error, error) {
	stream, err := c.session.OpenStream(ctx)
	if err != nil {
		return nil, nil, contextualIOError(ctx, "open watch stream", err)
	}
	stopCancellation := interruptStreamOnCancel(ctx, stream)
	requestID := uuid.NewString()
	request := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: &cnpv1.Request{
		Metadata:  &cnpv1.RequestMetadata{RequestId: requestID},
		Operation: &cnpv1.Request_WatchChanges{WatchChanges: &cnpv1.WatchChangesRequest{}},
	}}}
	if err := cnp.Write(stream, request); err != nil {
		stopCancellation()
		_ = stream.Close()
		stream.CancelRead()
		stream.CancelWrite()
		return nil, nil, contextualIOError(ctx, "write watch request", err)
	}
	events := make(chan *cnpv1.ChangeEvent, 1)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errorsOut)
		defer stream.Close()
		defer stopCancellation()
		reader := cnp.NewReader(stream)
		for {
			var message cnpv1.ServerMessage
			if err := reader.Read(&message); err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					errorsOut <- err
				}
				return
			}
			response := message.GetResponse()
			if response == nil || response.RequestId != requestID {
				errorsOut <- fmt.Errorf("invalid watch response")
				return
			}
			if status := response.GetError(); status != nil {
				errorsOut <- &Error{Code: status.Code, Message: status.Message}
				return
			}
			change := response.GetChange()
			if change == nil {
				errorsOut <- unexpectedResult(response)
				return
			}
			select {
			case events <- change:
			case <-ctx.Done():
				_ = stream.Close()
				stream.CancelRead()
				return
			}
		}
	}()
	return events, errorsOut, nil
}

func (c *Client) WatchJobOutput(ctx context.Context, jobExecutionID string, afterEventID int64) (<-chan *cnpv1.JobOutputBatch, <-chan error, error) {
	stream, err := c.session.OpenStream(ctx)
	if err != nil {
		return nil, nil, contextualIOError(ctx, "open job output stream", err)
	}
	stopCancellation := interruptStreamOnCancel(ctx, stream)
	requestID := uuid.NewString()
	request := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: &cnpv1.Request{
		Metadata: &cnpv1.RequestMetadata{RequestId: requestID},
		Operation: &cnpv1.Request_WatchJobOutput{WatchJobOutput: &cnpv1.WatchJobOutputRequest{
			JobExecutionId: jobExecutionID, AfterEventId: afterEventID,
		}},
	}}}
	if err := cnp.Write(stream, request); err != nil {
		stopCancellation()
		_ = stream.Close()
		stream.CancelRead()
		stream.CancelWrite()
		return nil, nil, contextualIOError(ctx, "write job output request", err)
	}
	batches := make(chan *cnpv1.JobOutputBatch, 1)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(batches)
		defer close(errorsOut)
		defer stream.Close()
		defer stopCancellation()
		reader := cnp.NewReader(stream)
		for {
			var message cnpv1.ServerMessage
			if err := reader.Read(&message); err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					errorsOut <- err
				}
				return
			}
			response := message.GetResponse()
			if response == nil || response.RequestId != requestID {
				errorsOut <- fmt.Errorf("invalid job output response")
				return
			}
			if status := response.GetError(); status != nil {
				errorsOut <- &Error{Code: status.Code, Message: status.Message}
				return
			}
			batch := response.GetJobOutput()
			if batch == nil {
				errorsOut <- unexpectedResult(response)
				return
			}
			select {
			case batches <- batch:
			case <-ctx.Done():
				_ = stream.Close()
				stream.CancelRead()
				return
			}
		}
	}()
	return batches, errorsOut, nil
}

func (c *Client) WatchJobLog(ctx context.Context, jobExecutionID string, afterChunkID int64) (<-chan *cnpv1.JobLogDescriptor, <-chan error, error) {
	stream, err := c.session.OpenStream(ctx)
	if err != nil {
		return nil, nil, contextualIOError(ctx, "open job log stream", err)
	}
	stopCancellation := interruptStreamOnCancel(ctx, stream)
	requestID := uuid.NewString()
	request := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: &cnpv1.Request{
		Metadata: &cnpv1.RequestMetadata{RequestId: requestID},
		Operation: &cnpv1.Request_WatchJobLog{WatchJobLog: &cnpv1.WatchJobLogRequest{
			JobExecutionId: jobExecutionID, AfterChunkId: afterChunkID,
		}},
	}}}
	if err := cnp.Write(stream, request); err != nil {
		stopCancellation()
		_ = stream.Close()
		stream.CancelRead()
		stream.CancelWrite()
		return nil, nil, contextualIOError(ctx, "write job log request", err)
	}
	descriptors := make(chan *cnpv1.JobLogDescriptor, 1)
	errorsOut := make(chan error, 1)
	go func() {
		defer close(descriptors)
		defer close(errorsOut)
		defer stream.Close()
		defer stopCancellation()
		reader := cnp.NewReader(stream)
		for {
			var message cnpv1.ServerMessage
			if err := reader.Read(&message); err != nil {
				if ctx.Err() == nil && !errors.Is(err, io.EOF) {
					errorsOut <- err
				}
				return
			}
			response := message.GetResponse()
			if response == nil || response.RequestId != requestID {
				errorsOut <- fmt.Errorf("invalid job log response")
				return
			}
			if status := response.GetError(); status != nil {
				errorsOut <- &Error{Code: status.Code, Message: status.Message}
				return
			}
			descriptor := response.GetJobLogDescriptor()
			if descriptor == nil {
				errorsOut <- unexpectedResult(response)
				return
			}
			select {
			case descriptors <- descriptor:
			case <-ctx.Done():
				_ = stream.Close()
				stream.CancelRead()
				return
			}
		}
	}()
	return descriptors, errorsOut, nil
}

func (c *Client) call(ctx context.Context, request *cnpv1.Request, idempotencyKey string) (*cnpv1.Response, error) {
	stream, err := c.session.OpenStream(ctx)
	if err != nil {
		return nil, contextualIOError(ctx, "open native request stream", err)
	}
	defer stream.Close()
	stopCancellation := interruptStreamOnCancel(ctx, stream)
	defer stopCancellation()
	requestID := uuid.NewString()
	metadata := &cnpv1.RequestMetadata{RequestId: requestID, IdempotencyKey: idempotencyKey}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 {
			metadata.TimeoutMs = uint32(min(remaining.Milliseconds(), int64(^uint32(0))))
		}
	}
	request.Metadata = metadata
	message := &cnpv1.ClientMessage{Body: &cnpv1.ClientMessage_Request{Request: request}}
	if err := cnp.Write(stream, message); err != nil {
		stream.CancelRead()
		return nil, contextualIOError(ctx, "write native request", err)
	}
	_ = stream.Close()
	var serverMessage cnpv1.ServerMessage
	if err := cnp.NewReader(stream).Read(&serverMessage); err != nil {
		return nil, contextualIOError(ctx, "read native response", err)
	}
	response := serverMessage.GetResponse()
	if response == nil || response.RequestId != requestID {
		return nil, fmt.Errorf("invalid native response")
	}
	if status := response.GetError(); status != nil {
		return nil, &Error{Code: status.Code, Message: status.Message}
	}
	if !stopCancellation() {
		return nil, fmt.Errorf("complete native request: %w", ctx.Err())
	}
	return response, nil
}

func interruptStreamOnCancel(ctx context.Context, stream cnp.Stream) func() bool {
	return context.AfterFunc(ctx, func() {
		stream.CancelRead()
		stream.CancelWrite()
		_ = stream.Close()
	})
}

func contextualIOError(ctx context.Context, operation string, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return fmt.Errorf("%s: %w", operation, contextErr)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func unexpectedResult(response *cnpv1.Response) error {
	return fmt.Errorf("unexpected native response result for request %q", response.GetRequestId())
}

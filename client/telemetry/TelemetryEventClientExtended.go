package telemetry

import (
	"encoding/json"
	"github.com/devtron-labs/devtron/internal/sql/repository"
	"github.com/devtron-labs/devtron/internal/sql/repository/app"
	"github.com/devtron-labs/devtron/internal/sql/repository/pipelineConfig"
	util2 "github.com/devtron-labs/devtron/internal/util"
	chartRepoRepository "github.com/devtron-labs/devtron/pkg/chartRepo/repository"
	"github.com/devtron-labs/devtron/pkg/cluster"
	moduleRepo "github.com/devtron-labs/devtron/pkg/module/repo"
	serverDataStore "github.com/devtron-labs/devtron/pkg/server/store"
	"github.com/devtron-labs/devtron/pkg/sso"
	"github.com/devtron-labs/devtron/pkg/user"
	util3 "github.com/devtron-labs/devtron/pkg/util"
	"github.com/devtron-labs/devtron/util"
	"github.com/go-pg/pg"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"net/http"
	"time"
)

const AppsCount int = 50

type TelemetryEventClientImplExtended struct {
	environmentService            cluster.EnvironmentService
	appListingRepository          repository.AppListingRepository
	ciPipelineRepository          pipelineConfig.CiPipelineRepository
	pipelineRepository            pipelineConfig.PipelineRepository
	gitOpsConfigRepository        repository.GitOpsConfigRepository
	gitProviderRepository         repository.GitProviderRepository
	dockerArtifactStoreRepository repository.DockerArtifactStoreRepository
	appRepository                 app.AppRepository
	ciWorkflowRepository          pipelineConfig.CiWorkflowRepository
	cdWorkflowRepository          pipelineConfig.CdWorkflowRepository
	materialRepository            pipelineConfig.MaterialRepository
	ciTemplateRepository          pipelineConfig.CiTemplateRepository
	chartRepository               chartRepoRepository.ChartRepository
	*TelemetryEventClientImpl
}

func NewTelemetryEventClientImplExtended(logger *zap.SugaredLogger, client *http.Client, clusterService cluster.ClusterService,
	K8sUtil *util2.K8sUtil, aCDAuthConfig *util3.ACDAuthConfig,
	environmentService cluster.EnvironmentService, userService user.UserService,
	appListingRepository repository.AppListingRepository, PosthogClient *PosthogClient,
	ciPipelineRepository pipelineConfig.CiPipelineRepository, pipelineRepository pipelineConfig.PipelineRepository,
	gitOpsConfigRepository repository.GitOpsConfigRepository, gitProviderRepository repository.GitProviderRepository,
	attributeRepo repository.AttributesRepository, ssoLoginService sso.SSOLoginService, appRepository app.AppRepository,
	ciWorkflowRepository pipelineConfig.CiWorkflowRepository, cdWorkflowRepository pipelineConfig.CdWorkflowRepository,
	dockerArtifactStoreRepository repository.DockerArtifactStoreRepository,
	materialRepository pipelineConfig.MaterialRepository, ciTemplateRepository pipelineConfig.CiTemplateRepository,
	chartRepository chartRepoRepository.ChartRepository, moduleRepository moduleRepo.ModuleRepository, serverDataStore *serverDataStore.ServerDataStore, userAuditService user.UserAuditService) (*TelemetryEventClientImplExtended, error) {

	cron := cron.New(
		cron.WithChain())
	cron.Start()
	watcher := &TelemetryEventClientImplExtended{
		environmentService:            environmentService,
		appListingRepository:          appListingRepository,
		ciPipelineRepository:          ciPipelineRepository,
		pipelineRepository:            pipelineRepository,
		gitOpsConfigRepository:        gitOpsConfigRepository,
		gitProviderRepository:         gitProviderRepository,
		dockerArtifactStoreRepository: dockerArtifactStoreRepository,
		appRepository:                 appRepository,
		cdWorkflowRepository:          cdWorkflowRepository,
		ciWorkflowRepository:          ciWorkflowRepository,
		materialRepository:            materialRepository,
		ciTemplateRepository:          ciTemplateRepository,
		chartRepository:               chartRepository,
		TelemetryEventClientImpl: &TelemetryEventClientImpl{
			cron:             cron,
			logger:           logger,
			client:           client,
			clusterService:   clusterService,
			K8sUtil:          K8sUtil,
			aCDAuthConfig:    aCDAuthConfig,
			userService:      userService,
			attributeRepo:    attributeRepo,
			ssoLoginService:  ssoLoginService,
			PosthogClient:    PosthogClient,
			moduleRepository: moduleRepository,
			serverDataStore:  serverDataStore,
			userAuditService: userAuditService,
		},
	}

	watcher.HeartbeatEventForTelemetry()
	_, err := cron.AddFunc(SummaryCronExpr, watcher.SummaryEventForTelemetry)
	if err != nil {
		logger.Errorw("error in starting summery event", "err", err)
		return nil, err
	}

	_, err = cron.AddFunc(HeartbeatCronExpr, watcher.HeartbeatEventForTelemetry)
	if err != nil {
		logger.Errorw("error in starting heartbeat event", "err", err)
		return nil, err
	}
	return watcher, err
}

type TelemetryEventDto struct {
	UCID                                 string             `json:"ucid"` //unique client id
	Timestamp                            time.Time          `json:"timestamp"`
	EventMessage                         string             `json:"eventMessage,omitempty"`
	EventType                            TelemetryEventType `json:"eventType"`
	ProdAppCount                         int                `json:"prodAppCount,omitempty"`
	NonProdAppCount                      int                `json:"nonProdAppCount,omitempty"`
	UserCount                            int                `json:"userCount,omitempty"`
	EnvironmentCount                     int                `json:"environmentCount,omitempty"`
	ClusterCount                         int                `json:"clusterCount,omitempty"`
	CiCountPerDay                        int                `json:"ciCountPerDay,omitempty"`
	CdCountPerDay                        int                `json:"cdCountPerDay,omitempty"`
	HelmChartCount                       int                `json:"helmChartCount,omitempty"`
	SecurityScanCountPerDay              int                `json:"securityScanCountPerDay,omitempty"`
	GitAccountsCount                     int                `json:"gitAccountsCount,omitempty"`
	GitOpsCount                          int                `json:"gitOpsCount,omitempty"`
	RegistryCount                        int                `json:"registryCount,omitempty"`
	HostURL                              bool               `json:"hostURL,omitempty"`
	SSOLogin                             bool               `json:"ssoLogin,omitempty"`
	AppCount                             int                `json:"appCount,omitempty"`
	AppsWithGitRepoConfigured            int                `json:"appsWithGitRepoConfigured,omitempty"`
	AppsWithDockerConfigured             int                `json:"appsWithDockerConfigured,omitempty"`
	AppsWithDeploymentTemplateConfigured int                `json:"appsWithDeploymentTemplateConfigured,omitempty"`
	AppsWithCiPipelineConfigured         int                `json:"appsWithCiPipelineConfigured,omitempty"`
	AppsWithCdPipelineConfigured         int                `json:"appsWithCdPipelineConfigured,omitempty"`
	Build                                bool               `json:"build,omitempty"`
	Deployment                           bool               `json:"deployment,omitempty"`
	ServerVersion                        string             `json:"serverVersion,omitempty"`
	DevtronGitVersion                    string             `json:"devtronGitVersion,omitempty"`
	DevtronVersion                       string             `json:"devtronVersion,omitempty"`
	DevtronMode                          string             `json:"devtronMode,omitempty"`
	InstalledIntegrations                []string           `json:"installedIntegrations,omitempty"`
	InstallFailedIntegrations            []string           `json:"installFailedIntegrations,omitempty"`
	InstallTimedOutIntegrations          []string           `json:"installTimedOutIntegrations,omitempty"`
	InstallingIntegrations               []string           `json:"installingIntegrations,omitempty"`
	DevtronReleaseVersion                string             `json:"devtronReleaseVersion,omitempty"`
	LastLoginTime                        time.Time          `json:"LastLoginTime,omitempty"`
}

func (impl *TelemetryEventClientImplExtended) SummaryEventForTelemetry() {
	err := impl.SendSummaryEvent(string(Summary))
	if err != nil {
		impl.logger.Errorw("error occurred in SummaryEventForTelemetry", "err", err)
	}
}

func (impl *TelemetryEventClientImplExtended) SendSummaryEvent(eventType string) error {
	impl.logger.Infow("sending summary event", "eventType", eventType)
	ucid, err := impl.getUCID()
	if err != nil {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	if IsOptOut {
		impl.logger.Warnw("client is opt-out for telemetry, there will be no events capture", "ucid", ucid)
		return err
	}

	clusters, users, k8sServerVersion, hostURL, ssoSetup := impl.SummaryDetailsForTelemetry()
	payload := &TelemetryEventDto{UCID: ucid, Timestamp: time.Now(), EventType: TelemetryEventType(eventType), DevtronVersion: "v1"}
	payload.ServerVersion = k8sServerVersion.String()

	environments, err := impl.environmentService.GetAllActive()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	prodApps, err := impl.appListingRepository.FindAppCount(true)
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	nonProdApps, err := impl.appListingRepository.FindAppCount(false)
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	ciPipeline, err := impl.ciPipelineRepository.FindAllPipelineInLast24Hour()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	cdPipeline, err := impl.pipelineRepository.FindAllPipelineInLast24Hour()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	gitAccounts, err := impl.gitProviderRepository.FindAll()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	gitOps, err := impl.gitOpsConfigRepository.GetAllGitOpsConfig()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	containerRegistry, err := impl.dockerArtifactStoreRepository.FindAll()
	if err != nil && err != pg.ErrNoRows {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	//appSetup := false
	apps, err := impl.appRepository.FindAll()
	if err != nil {
		impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		return err
	}

	var appIds []int
	for _, appInfo := range apps {
		appIds = append(appIds, appInfo.Id)
	}

	payload.AppCount = len(appIds)
	if len(appIds) < AppsCount {
		payload.AppsWithGitRepoConfigured, err = impl.materialRepository.FindNumberOfAppsWithGitRepo(appIds)
		if err != nil {
			impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		}
		payload.AppsWithDockerConfigured, err = impl.ciTemplateRepository.FindNumberOfAppsWithDockerConfigured(appIds)
		if err != nil {
			impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		}
		payload.AppsWithDeploymentTemplateConfigured, err = impl.chartRepository.FindNumberOfAppsWithDeploymentTemplate(appIds)
		if err != nil {
			impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		}
		payload.AppsWithCiPipelineConfigured, err = impl.ciPipelineRepository.FindNumberOfAppsWithCiPipeline(appIds)
		if err != nil {
			impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		}

		payload.AppsWithCdPipelineConfigured, err = impl.pipelineRepository.FindNumberOfAppsWithCdPipeline(appIds)
		if err != nil {
			impl.logger.Errorw("exception caught inside telemetry summary event", "err", err)
		}
	}

	build, err := impl.ciWorkflowRepository.ExistsByStatus("Succeeded")

	deployment, err := impl.cdWorkflowRepository.ExistsByStatus("Healthy")

	// build integrations data
	installedIntegrations, installFailedIntegrations, installTimedOutIntegrations, installingIntegrations, err := impl.buildIntegrationsList()
	if err != nil {
		return err
	}

	devtronVersion := util.GetDevtronVersion()
	payload.ProdAppCount = prodApps
	payload.NonProdAppCount = nonProdApps
	payload.RegistryCount = len(containerRegistry)
	payload.SSOLogin = ssoSetup
	payload.UserCount = len(users)
	payload.EnvironmentCount = len(environments)
	payload.ClusterCount = len(clusters)
	payload.CiCountPerDay = len(ciPipeline)
	payload.CdCountPerDay = len(cdPipeline)
	payload.GitAccountsCount = len(gitAccounts)
	payload.GitOpsCount = len(gitOps)
	payload.HostURL = hostURL
	payload.DevtronGitVersion = devtronVersion.GitCommit
	payload.Build = build
	payload.Deployment = deployment
	payload.DevtronMode = devtronVersion.ServerMode
	payload.InstalledIntegrations = installedIntegrations
	payload.InstallFailedIntegrations = installFailedIntegrations
	payload.InstallTimedOutIntegrations = installTimedOutIntegrations
	payload.InstallingIntegrations = installingIntegrations
	payload.DevtronReleaseVersion = impl.serverDataStore.CurrentVersion

	latestUser, err := impl.userAuditService.GetLatestUser()
	if err == nil {
		loginTime := latestUser.UpdatedOn
		if loginTime.IsZero() {
			loginTime = latestUser.CreatedOn
		}
		payload.LastLoginTime = loginTime
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		impl.logger.Errorw("SummaryEventForTelemetry, payload marshal error", "error", err)
		return err
	}
	prop := make(map[string]interface{})
	err = json.Unmarshal(reqBody, &prop)
	if err != nil {
		impl.logger.Errorw("SummaryEventForTelemetry, payload unmarshal error", "error", err)
		return err
	}

	err = impl.EnqueuePostHog(ucid, TelemetryEventType(eventType), prop)
	if err != nil {
		impl.logger.Errorw("SummaryEventForTelemetry, failed to push event", "ucid", ucid, "error", err)
		return err
	}
	return nil
}

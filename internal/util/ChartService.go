/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package util

import (
	"compress/gzip"
	"context"
	"fmt"
	repository3 "github.com/argoproj/argo-cd/v2/pkg/apiclient/repository"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	repository4 "github.com/devtron-labs/devtron/client/argocdServer/repository"
	"github.com/devtron-labs/devtron/internal/sql/repository"
	appStoreBean "github.com/devtron-labs/devtron/pkg/appStore/bean"
	repository2 "github.com/devtron-labs/devtron/pkg/user/repository"
	"github.com/devtron-labs/devtron/util"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ghodss/yaml"
	dirCopy "github.com/otiai10/copy"
	"go.uber.org/zap"
	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/proto/hapi/chart"
)

type ChartWorkingDir string

const PIPELINE_DEPLOYMENT_TYPE_ACD string = "argo_cd"
const PIPELINE_DEPLOYMENT_TYPE_HELM string = "helm"

type ChartTemplateService interface {
	FetchValuesFromReferenceChart(chartMetaData *chart.Metadata, refChartLocation string, templateName string, userId int32) (*ChartValues, *ChartGitAttribute, error)
	GetChartVersion(location string) (string, error)
	CreateChartProxy(chartMetaData *chart.Metadata, refChartLocation string, templateName string, version string, envName string, installAppVersionRequest *appStoreBean.InstallAppVersionDTO) (string, *ChartGitAttribute, error)
	GitPull(clonedDir string, repoUrl string, appStoreName string) error
	GetDir() string
	GetUserEmailIdAndNameForGitOpsCommit(userId int32) (emailId, name string)
	GetGitOpsRepoName(appName string) string
	GetGitOpsRepoNameFromUrl(gitRepoUrl string) string
	CreateGitRepositoryForApp(gitOpsRepoName, baseTemplateName, version string, userId int32) (chartGitAttribute *ChartGitAttribute, err error)
	RegisterInArgo(chartGitAttribute *ChartGitAttribute, ctx context.Context) error
	BuildChartAndPushToGitRepo(chartMetaData *chart.Metadata, referenceTemplatePath string, gitOpsRepoName, referenceTemplate, version, repoUrl string, userId int32) error
	GetByteArrayRefChart(chartMetaData *chart.Metadata, referenceTemplatePath string) ([]byte, error)
	CreateReadmeInGitRepo(gitOpsRepoName string, userId int32) error
}
type ChartTemplateServiceImpl struct {
	randSource             rand.Source
	logger                 *zap.SugaredLogger
	chartWorkingDir        ChartWorkingDir
	gitFactory             *GitFactory
	client                 *http.Client
	globalEnvVariables     *util.GlobalEnvVariables
	gitOpsConfigRepository repository.GitOpsConfigRepository
	userRepository         repository2.UserRepository
	repositoryService      repository4.ServiceClient
}

type ChartValues struct {
	Values                  string `json:"values"`            //yaml
	AppOverrides            string `json:"appOverrides"`      //json
	EnvOverrides            string `json:"envOverrides"`      //json
	ReleaseOverrides        string `json:"releaseOverrides"`  //json
	PipelineOverrides       string `json:"pipelineOverrides"` //json
	ImageDescriptorTemplate string `json:"-"`
}

func NewChartTemplateServiceImpl(logger *zap.SugaredLogger,
	chartWorkingDir ChartWorkingDir,
	client *http.Client,
	gitFactory *GitFactory, globalEnvVariables *util.GlobalEnvVariables,
	gitOpsConfigRepository repository.GitOpsConfigRepository,
	userRepository repository2.UserRepository, repositoryService repository4.ServiceClient) *ChartTemplateServiceImpl {
	return &ChartTemplateServiceImpl{
		randSource:             rand.NewSource(time.Now().UnixNano()),
		logger:                 logger,
		chartWorkingDir:        chartWorkingDir,
		client:                 client,
		gitFactory:             gitFactory,
		globalEnvVariables:     globalEnvVariables,
		gitOpsConfigRepository: gitOpsConfigRepository,
		userRepository:         userRepository,
		repositoryService:      repositoryService,
	}
}
func (impl ChartTemplateServiceImpl) RegisterInArgo(chartGitAttribute *ChartGitAttribute, ctx context.Context) error {
	repo := &v1alpha1.Repository{
		Repo: chartGitAttribute.RepoUrl,
	}
	repo, err := impl.repositoryService.Create(ctx, &repository3.RepoCreateRequest{Repo: repo, Upsert: true})
	if err != nil {
		impl.logger.Errorw("error in creating argo Repository ", "err", err)
		return err
	}
	impl.logger.Infow("repo registered in argo", "name", chartGitAttribute.RepoUrl)
	return err
}

func (impl ChartTemplateServiceImpl) GetChartVersion(location string) (string, error) {
	if fi, err := os.Stat(location); err != nil {
		return "", err
	} else if !fi.IsDir() {
		return "", fmt.Errorf("%q is not a directory", location)
	}

	chartYaml := filepath.Join(location, "Chart.yaml")
	if _, err := os.Stat(chartYaml); os.IsNotExist(err) {
		return "", fmt.Errorf("Chart.yaml file not present in the directory %q", location)
	}
	//chartYaml = filepath.Join(chartYaml,filepath.Clean(chartYaml))
	chartYamlContent, err := ioutil.ReadFile(filepath.Clean(chartYaml))
	if err != nil {
		return "", fmt.Errorf("cannot read Chart.Yaml in directory %q", location)
	}

	chartContent, err := chartutil.UnmarshalChartfile(chartYamlContent)
	if err != nil {
		return "", fmt.Errorf("cannot read Chart.Yaml in directory %q", location)
	}

	return chartContent.Version, nil
}

func (impl ChartTemplateServiceImpl) FetchValuesFromReferenceChart(chartMetaData *chart.Metadata, refChartLocation string, templateName string, userId int32) (*ChartValues, *ChartGitAttribute, error) {
	chartMetaData.ApiVersion = "v1" // ensure always v1
	dir := impl.GetDir()
	chartDir := filepath.Join(string(impl.chartWorkingDir), dir)
	impl.logger.Debugw("chart dir ", "chart", chartMetaData.Name, "dir", chartDir)
	err := os.MkdirAll(chartDir, os.ModePerm) //hack for concurrency handling
	if err != nil {
		impl.logger.Errorw("err in creating dir", "dir", chartDir, "err", err)
		return nil, nil, err
	}

	defer impl.CleanDir(chartDir)
	err = dirCopy.Copy(refChartLocation, chartDir)

	if err != nil {
		impl.logger.Errorw("error in copying chart for app", "app", chartMetaData.Name, "error", err)
		return nil, nil, err
	}
	archivePath, valuesYaml, err := impl.packageChart(chartDir, chartMetaData)
	if err != nil {
		impl.logger.Errorw("error in creating archive", "err", err)
		return nil, nil, err
	}
	values, err := impl.getValues(chartDir)
	if err != nil {
		impl.logger.Errorw("error in pushing chart", "path", archivePath, "err", err)
		return nil, nil, err
	}
	values.Values = valuesYaml
	descriptor, err := ioutil.ReadFile(filepath.Clean(filepath.Join(chartDir, ".image_descriptor_template.json")))
	if err != nil {
		impl.logger.Errorw("error in reading descriptor", "path", chartDir, "err", err)
		return nil, nil, err
	}
	values.ImageDescriptorTemplate = string(descriptor)
	chartGitAttr := &ChartGitAttribute{}
	return values, chartGitAttr, nil
}

func (impl ChartTemplateServiceImpl) BuildChartAndPushToGitRepo(chartMetaData *chart.Metadata, referenceTemplatePath string, gitOpsRepoName, referenceTemplate, version, repoUrl string, userId int32) error {
	impl.logger.Debugw("package chart and push to git", "gitOpsRepoName", gitOpsRepoName, "version", version, "referenceTemplate", referenceTemplate, "repoUrl", repoUrl)
	chartMetaData.ApiVersion = "v1" // ensure always v1
	dir := impl.GetDir()
	tempReferenceTemplateDir := filepath.Join(string(impl.chartWorkingDir), dir)
	impl.logger.Debugw("chart dir ", "chart", chartMetaData.Name, "dir", tempReferenceTemplateDir)
	err := os.MkdirAll(tempReferenceTemplateDir, os.ModePerm) //hack for concurrency handling
	if err != nil {
		impl.logger.Errorw("err in creating dir", "dir", tempReferenceTemplateDir, "err", err)
		return err
	}
	defer impl.CleanDir(tempReferenceTemplateDir)
	err = dirCopy.Copy(referenceTemplatePath, tempReferenceTemplateDir)

	if err != nil {
		impl.logger.Errorw("error in copying chart for app", "app", chartMetaData.Name, "error", err)
		return err
	}
	_, _, err = impl.packageChart(tempReferenceTemplateDir, chartMetaData)
	if err != nil {
		impl.logger.Errorw("error in creating archive", "err", err)
		return err
	}

	err = impl.pushChartToGitRepo(gitOpsRepoName, referenceTemplate, version, tempReferenceTemplateDir, repoUrl, userId)
	if err != nil {
		impl.logger.Errorw("error in pushing chart to git ", "err", err)
		return err
	}
	return nil
}

type ChartGitAttribute struct {
	RepoUrl, ChartLocation string
}

func (impl ChartTemplateServiceImpl) CreateGitRepositoryForApp(gitOpsRepoName, baseTemplateName, version string, userId int32) (chartGitAttribute *ChartGitAttribute, err error) {
	//baseTemplateName  replace whitespace
	space := regexp.MustCompile(`\s+`)
	gitOpsRepoName = space.ReplaceAllString(gitOpsRepoName, "-")

	//getting user name & emailId for commit author data
	userEmailId, userName := impl.GetUserEmailIdAndNameForGitOpsCommit(userId)
	repoUrl, _, detailedError := impl.gitFactory.Client.CreateRepository(gitOpsRepoName, fmt.Sprintf("helm chart for "+gitOpsRepoName), userName, userEmailId)
	for _, err := range detailedError.StageErrorMap {
		if err != nil {
			impl.logger.Errorw("error in creating git project", "name", gitOpsRepoName, "err", err)
			return nil, err
		}
	}
	return &ChartGitAttribute{RepoUrl: repoUrl, ChartLocation: filepath.Join(baseTemplateName, version)}, nil
}

func (impl ChartTemplateServiceImpl) pushChartToGitRepo(gitOpsRepoName, referenceTemplate, version, tempReferenceTemplateDir string, repoUrl string, userId int32) (err error) {
	chartDir := fmt.Sprintf("%s-%s", gitOpsRepoName, impl.GetDir())
	clonedDir := impl.gitFactory.gitService.GetCloneDirectory(chartDir)
	if _, err := os.Stat(clonedDir); os.IsNotExist(err) {
		clonedDir, err = impl.gitFactory.gitService.Clone(repoUrl, chartDir)
		if err != nil {
			impl.logger.Errorw("error in cloning repo", "url", repoUrl, "err", err)
			return err
		}
	} else {
		err = impl.GitPull(clonedDir, repoUrl, gitOpsRepoName)
		if err != nil {
			impl.logger.Errorw("error in pulling git repo", "url", repoUrl, "err", err)
			return err
		}
	}

	dir := filepath.Join(clonedDir, referenceTemplate, version)
	pushChartToGit := true

	//if chart already exists don't overrides it by reference template
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			impl.logger.Errorw("error in making dir", "err", err)
			return err
		}
		err = dirCopy.Copy(tempReferenceTemplateDir, dir)
		if err != nil {
			impl.logger.Errorw("error copying dir", "err", err)
			return err
		}
	} else {
		// auto-healing : data corruption fix - sometimes reference chart contents are not pushed in git-ops repo.
		// copying content from reference template dir to cloned dir (if Chart.yaml file is not found)
		// if Chart.yaml file is not found, we are assuming here that reference chart contents are not pushed in git-ops repo
		if _, err := os.Stat(filepath.Join(dir, "Chart.yaml")); os.IsNotExist(err) {
			impl.logger.Infow("auto-healing: Chart.yaml not found in cloned repo from git-ops. copying content", "from", tempReferenceTemplateDir, "to", dir)
			err = dirCopy.Copy(tempReferenceTemplateDir, dir)
			if err != nil {
				impl.logger.Errorw("error copying content in auto-healing", "err", err)
				return err
			}
		} else {
			// chart exists on git, hence not performing first commit
			pushChartToGit = false
		}
	}

	// if push needed, then only push
	if pushChartToGit {
		userEmailId, userName := impl.GetUserEmailIdAndNameForGitOpsCommit(userId)
		commit, err := impl.gitFactory.gitService.CommitAndPushAllChanges(clonedDir, "first commit", userName, userEmailId)
		if err != nil {
			impl.logger.Errorw("error in pushing git", "err", err)
			impl.logger.Warn("re-trying, taking pull and then push again")
			err = impl.GitPull(clonedDir, repoUrl, gitOpsRepoName)
			if err != nil {
				return err
			}
			err = dirCopy.Copy(tempReferenceTemplateDir, dir)
			if err != nil {
				impl.logger.Errorw("error copying dir", "err", err)
				return err
			}
			commit, err = impl.gitFactory.gitService.CommitAndPushAllChanges(clonedDir, "first commit", userName, userEmailId)
			if err != nil {
				impl.logger.Errorw("error in pushing git", "err", err)
				return err
			}
		}
		impl.logger.Debugw("template committed", "url", repoUrl, "commit", commit)
	}

	defer impl.CleanDir(clonedDir)
	return nil
}

func (impl ChartTemplateServiceImpl) getValues(directory string) (values *ChartValues, err error) {

	if fi, err := os.Stat(directory); err != nil {
		return nil, err
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("%q is not a directory", directory)
	}

	files, err := ioutil.ReadDir(directory)
	if err != nil {
		impl.logger.Errorw("failed reading directory", "err", err)
		return nil, fmt.Errorf(" Couldn't read the %q", directory)
	}

	var appOverrideByte, envOverrideByte, releaseOverrideByte, pipelineOverrideByte []byte

	for _, file := range files {
		if !file.IsDir() {
			name := strings.ToLower(file.Name())
			if name == "app-values.yaml" || name == "app-values.yml" {
				appOverrideByte, err = ioutil.ReadFile(filepath.Clean(filepath.Join(directory, file.Name())))
				if err != nil {
					impl.logger.Errorw("failed reading data from file", "err", err)
				} else {
					appOverrideByte, err = yaml.YAMLToJSON(appOverrideByte)
					if err != nil {
						return nil, err
					}
				}
			}
			if name == "env-values.yaml" || name == "env-values.yml" {
				envOverrideByte, err = ioutil.ReadFile(filepath.Clean(filepath.Join(directory, file.Name())))
				if err != nil {
					impl.logger.Errorw("failed reading data from file", "err", err)
				} else {
					envOverrideByte, err = yaml.YAMLToJSON(envOverrideByte)
					if err != nil {
						return nil, err
					}
				}
			}
			if name == "release-values.yaml" || name == "release-values.yml" {
				releaseOverrideByte, err = ioutil.ReadFile(filepath.Clean(filepath.Join(directory, file.Name())))
				if err != nil {
					impl.logger.Errorw("failed reading data from file", "err", err)
				} else {
					releaseOverrideByte, err = yaml.YAMLToJSON(releaseOverrideByte)
					if err != nil {
						return nil, err
					}
				}
			}
			if name == "pipeline-values.yaml" || name == "pipeline-values.yml" {
				pipelineOverrideByte, err = ioutil.ReadFile(filepath.Clean(filepath.Join(directory, file.Name())))
				if err != nil {
					impl.logger.Errorw("failed reading data from file", "err", err)
				} else {
					pipelineOverrideByte, err = yaml.YAMLToJSON(pipelineOverrideByte)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	val := &ChartValues{
		AppOverrides:      string(appOverrideByte),
		EnvOverrides:      string(envOverrideByte),
		ReleaseOverrides:  string(releaseOverrideByte),
		PipelineOverrides: string(pipelineOverrideByte),
	}
	return val, nil

}

func (impl ChartTemplateServiceImpl) packageChart(tempReferenceTemplateDir string, chartMetaData *chart.Metadata) (*string, string, error) {
	valid, err := chartutil.IsChartDir(tempReferenceTemplateDir)
	if err != nil {
		impl.logger.Errorw("error in validating base chart", "dir", tempReferenceTemplateDir, "err", err)
		return nil, "", err
	}
	if !valid {
		impl.logger.Errorw("invalid chart at ", "dir", tempReferenceTemplateDir)
		return nil, "", fmt.Errorf("invalid base chart")
	}

	b, err := yaml.Marshal(chartMetaData)
	if err != nil {
		impl.logger.Errorw("error in marshaling chartMetadata", "err", err)
		return nil, "", err
	}
	err = ioutil.WriteFile(filepath.Join(tempReferenceTemplateDir, "Chart.yaml"), b, 0600)
	if err != nil {
		impl.logger.Errorw("err in writing Chart.yaml", "err", err)
		return nil, "", err
	}
	chart, err := chartutil.LoadDir(tempReferenceTemplateDir)
	if err != nil {
		impl.logger.Errorw("error in loading chart dir", "err", err, "dir", tempReferenceTemplateDir)
		return nil, "", err
	}

	archivePath, err := chartutil.Save(chart, tempReferenceTemplateDir)
	if err != nil {
		impl.logger.Errorw("error in saving", "err", err, "dir", tempReferenceTemplateDir)
		return nil, "", err
	}
	impl.logger.Debugw("chart archive path", "path", archivePath)
	//chart.Values
	valuesYaml := chart.Values.Raw
	return &archivePath, valuesYaml, nil
}

func (impl ChartTemplateServiceImpl) CleanDir(dir string) {
	err := os.RemoveAll(dir)
	if err != nil {
		impl.logger.Warnw("error in deleting dir ", "dir", dir)
	}
}

func (impl ChartTemplateServiceImpl) GetDir() string {
	/* #nosec */
	r1 := rand.New(impl.randSource).Int63()
	return strconv.FormatInt(r1, 10)
}

func (impl ChartTemplateServiceImpl) CreateChartProxy(chartMetaData *chart.Metadata, refChartLocation string, templateName string, version string, envName string, installAppVersionRequest *appStoreBean.InstallAppVersionDTO) (string, *ChartGitAttribute, error) {
	chartMetaData.ApiVersion = "v2" // ensure always v2
	dir := impl.GetDir()
	chartDir := filepath.Join(string(impl.chartWorkingDir), dir)
	impl.logger.Debugw("chart dir ", "chart", chartMetaData.Name, "dir", chartDir)
	err := os.MkdirAll(chartDir, os.ModePerm) //hack for concurrency handling
	if err != nil {
		impl.logger.Errorw("err in creating dir", "dir", chartDir, "err", err)
		return "", nil, err
	}
	defer impl.CleanDir(chartDir)
	err = dirCopy.Copy(refChartLocation, chartDir)

	if err != nil {
		impl.logger.Errorw("error in copying chart for app", "app", chartMetaData.Name, "error", err)
		return "", nil, err
	}
	archivePath, valuesYaml, err := impl.packageChart(chartDir, chartMetaData)
	if err != nil {
		impl.logger.Errorw("error in creating archive", "err", err)
		return "", nil, err
	}

	chartGitAttr, err := impl.createAndPushToGitChartProxy(chartMetaData.Name, templateName, version, chartDir, envName, installAppVersionRequest)
	if err != nil {
		impl.logger.Errorw("error in pushing chart to git ", "path", archivePath, "err", err)
		return "", nil, err
	}
	if valuesYaml == "" {
		valuesYaml = "{}"
	} else {
		valuesYamlByte, err := yaml.YAMLToJSON([]byte(valuesYaml))
		if err != nil {
			return "", nil, err
		}
		valuesYaml = string(valuesYamlByte)
	}
	return valuesYaml, chartGitAttr, nil
}

func (impl ChartTemplateServiceImpl) createAndPushToGitChartProxy(appStoreName, baseTemplateName, version, tmpChartLocation string, envName string, installAppVersionRequest *appStoreBean.InstallAppVersionDTO) (chartGitAttribute *ChartGitAttribute, err error) {
	//baseTemplateName  replace whitespace
	space := regexp.MustCompile(`\s+`)
	appStoreName = space.ReplaceAllString(appStoreName, "-")

	if len(installAppVersionRequest.GitOpsRepoName) == 0 {
		//here git ops repo will be the app name, to breaking the mono repo structure
		gitOpsRepoName := impl.GetGitOpsRepoName(installAppVersionRequest.AppName)
		installAppVersionRequest.GitOpsRepoName = gitOpsRepoName
	}
	//getting user name & emailId for commit author data
	userEmailId, userName := impl.GetUserEmailIdAndNameForGitOpsCommit(installAppVersionRequest.UserId)
	repoUrl, _, detailedError := impl.gitFactory.Client.CreateRepository(installAppVersionRequest.GitOpsRepoName, "helm chart for "+installAppVersionRequest.GitOpsRepoName, userName, userEmailId)
	for _, err := range detailedError.StageErrorMap {
		if err != nil {
			impl.logger.Errorw("error in creating git project", "name", installAppVersionRequest.GitOpsRepoName, "err", err)
			return nil, err
		}
	}

	chartDir := fmt.Sprintf("%s-%s", installAppVersionRequest.AppName, impl.GetDir())
	clonedDir := impl.gitFactory.gitService.GetCloneDirectory(chartDir)
	if _, err := os.Stat(clonedDir); os.IsNotExist(err) {
		clonedDir, err = impl.gitFactory.gitService.Clone(repoUrl, chartDir)
		if err != nil {
			impl.logger.Errorw("error in cloning repo", "url", repoUrl, "err", err)
			return nil, err
		}
	} else {
		err = impl.GitPull(clonedDir, repoUrl, appStoreName)
		if err != nil {
			return nil, err
		}
	}

	acdAppName := fmt.Sprintf("%s-%s", installAppVersionRequest.AppName, envName)
	dir := filepath.Join(clonedDir, acdAppName)
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		impl.logger.Errorw("error in making dir", "err", err)
		return nil, err
	}
	err = dirCopy.Copy(tmpChartLocation, dir)
	if err != nil {
		impl.logger.Errorw("error copying dir", "err", err)
		return nil, err
	}
	commit, err := impl.gitFactory.gitService.CommitAndPushAllChanges(clonedDir, "first commit", userName, userEmailId)
	if err != nil {
		impl.logger.Errorw("error in pushing git", "err", err)
		impl.logger.Warn("re-trying, taking pull and then push again")
		err = impl.GitPull(clonedDir, repoUrl, acdAppName)
		if err != nil {
			return nil, err
		}
		err = dirCopy.Copy(tmpChartLocation, dir)
		if err != nil {
			impl.logger.Errorw("error copying dir", "err", err)
			return nil, err
		}
		commit, err = impl.gitFactory.gitService.CommitAndPushAllChanges(clonedDir, "first commit", userName, userEmailId)
		if err != nil {
			impl.logger.Errorw("error in pushing git", "err", err)
			return nil, err
		}
	}
	impl.logger.Debugw("template committed", "url", repoUrl, "commit", commit)
	defer impl.CleanDir(clonedDir)
	return &ChartGitAttribute{RepoUrl: repoUrl, ChartLocation: filepath.Join("", acdAppName)}, nil
}

func (impl ChartTemplateServiceImpl) GitPull(clonedDir string, repoUrl string, appStoreName string) error {
	err := impl.gitFactory.gitService.Pull(clonedDir) //TODO check for local repo exists before clone
	if err != nil {
		impl.logger.Errorw("error in pulling git", "clonedDir", clonedDir, "err", err)
		_, err := impl.gitFactory.gitService.Clone(repoUrl, appStoreName)
		if err != nil {
			impl.logger.Errorw("error in cloning repo", "url", repoUrl, "err", err)
			return err
		}
		return nil
	}
	return nil
}

func (impl *ChartTemplateServiceImpl) GetUserEmailIdAndNameForGitOpsCommit(userId int32) (string, string) {
	emailId := "devtron-bot@devtron.ai"
	name := "devtron bot"
	//getting emailId associated with user
	userDetail, _ := impl.userRepository.GetById(userId)
	if userDetail != nil && userDetail.EmailId != "admin" && userDetail.EmailId != "system" && len(userDetail.EmailId) > 0 {
		emailId = userDetail.EmailId
	} else {
		emailIdGitOps, err := impl.gitOpsConfigRepository.GetEmailIdFromActiveGitOpsConfig()
		if err != nil {
			impl.logger.Errorw("error in getting emailId from active gitOps config", "err", err)
		} else if len(emailIdGitOps) > 0 {
			emailId = emailIdGitOps
		}
	}
	//we are getting name from emailId(replacing special characters in <user-name part of email> with space)
	emailComponents := strings.Split(emailId, "@")
	regex, _ := regexp.Compile(`[^\w]`)
	if regex != nil {
		name = regex.ReplaceAllString(emailComponents[0], " ")
	}
	return emailId, name
}

func (impl ChartTemplateServiceImpl) GetGitOpsRepoName(appName string) string {
	var repoName string
	if len(impl.globalEnvVariables.GitOpsRepoPrefix) == 0 {
		repoName = appName
	} else {
		repoName = fmt.Sprintf("%s-%s", impl.globalEnvVariables.GitOpsRepoPrefix, appName)
	}
	return repoName
}

func (impl ChartTemplateServiceImpl) GetGitOpsRepoNameFromUrl(gitRepoUrl string) string {
	gitRepoUrl = gitRepoUrl[strings.LastIndex(gitRepoUrl, "/")+1:]
	gitRepoUrl = strings.ReplaceAll(gitRepoUrl, ".git", "")
	return gitRepoUrl
}

// GetByteArrayRefChart this method will be used for getting byte array from reference chart to store in db
func (impl ChartTemplateServiceImpl) GetByteArrayRefChart(chartMetaData *chart.Metadata, referenceTemplatePath string) ([]byte, error) {
	chartMetaData.ApiVersion = "v1" // ensure always v1
	dir := impl.GetDir()
	tempReferenceTemplateDir := filepath.Join(string(impl.chartWorkingDir), dir)
	impl.logger.Debugw("chart dir ", "chart", chartMetaData.Name, "dir", tempReferenceTemplateDir)
	err := os.MkdirAll(tempReferenceTemplateDir, os.ModePerm) //hack for concurrency handling
	if err != nil {
		impl.logger.Errorw("err in creating dir", "dir", tempReferenceTemplateDir, "err", err)
		return nil, err
	}
	defer impl.CleanDir(tempReferenceTemplateDir)
	err = dirCopy.Copy(referenceTemplatePath, tempReferenceTemplateDir)
	if err != nil {
		impl.logger.Errorw("error in copying chart for app", "app", chartMetaData.Name, "error", err)
		return nil, err
	}
	activePath, _, err := impl.packageChart(tempReferenceTemplateDir, chartMetaData)
	if err != nil {
		impl.logger.Errorw("error in creating archive", "err", err)
		return nil, err
	}
	file, err := os.Open(*activePath)
	reader, err := gzip.NewReader(file)
	if err != nil {
		impl.logger.Errorw("There is a problem with os.Open", "err", err)
		return nil, err
	}
	// read the complete content of the file h.Name into the bs []byte
	bs, err := ioutil.ReadAll(reader)
	if err != nil {
		impl.logger.Errorw("There is a problem with readAll", "err", err)
		return nil, err
	}
	return bs, nil
}

func (impl ChartTemplateServiceImpl) CreateReadmeInGitRepo(gitOpsRepoName string, userId int32) error {
	userEmailId, userName := impl.GetUserEmailIdAndNameForGitOpsCommit(userId)
	_, err := impl.gitFactory.Client.CreateReadme(gitOpsRepoName, userName, userEmailId)
	if err != nil {
		return err
	}
	return nil
}

func IsHelmApp(deploymentAppType string) bool {
	return deploymentAppType == PIPELINE_DEPLOYMENT_TYPE_HELM
}

func IsAcdApp(deploymentAppType string) bool {
	return deploymentAppType == PIPELINE_DEPLOYMENT_TYPE_ACD
}

package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
)

var client *gitlab.Client

func init() {
	var err error
	client, err = gitlab.NewClient(config.GitLab.AccessToken, gitlab.WithBaseURL(config.GitLab.Url))
	if err != nil {
		log.Panicln("failed to init gitlab client")
	}
}

func getEffectiveProjects() []*gitlab.Project {
	var projects = []*gitlab.Project{}

	configProjects := config.GitLab.Projects
	if len(configProjects) > 0 {
		for _, p := range configProjects {
			log := log.WithFields(log.Fields{"projectID": p.ID, "projectPath": p.Path})
			project, _, err := client.Projects.GetProject(p.ID, nil)
			if err != nil {
				log.Panicln("failed to get project info")
			}
			user, _, err := client.Users.CurrentUser()
			if err != nil {
				log.Panicln("failed to get user info")
			}
			if !isUserProjectMaintainer(user.ID, project.ID) {
				log.Panicln("current user is not the maintainer of project")
			}
			projects = append(projects, project)
		}
	} else {
		return listMaintainedProjects()
	}

	return projects
}

func listMaintainedProjects() []*gitlab.Project {
	var maintainedProjects = []*gitlab.Project{}

	user, _, err := client.Users.CurrentUser()
	if err != nil {
		log.Panicln("failed to get user info")
	}

	projects := listAllProjects()
	for _, p := range projects {
		if isUserProjectMaintainer(user.ID, p.ID) {
			maintainedProjects = append(maintainedProjects, p)
		}
	}

	return maintainedProjects
}

func isUserProjectMaintainer(userID int, projectID int) bool {
	maintainers := listProjectMaintainers(projectID)
	for _, m := range maintainers {
		if userID == m.ID {
			return true
		}
	}
	return false
}

func listAllProjects() []*gitlab.Project {
	var (
		projects = []*gitlab.Project{}
		options  = &gitlab.ListProjectsOptions{Archived: gitlab.Bool(false)}
	)

	for {
		projs, resp, err := client.Projects.ListProjects(options)
		if err != nil {
			log.Errorln("failed to get project list")
		}

		projects = append(projects, projs...)
		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return projects
}

func processProjectMergeRequests(project *gitlab.Project) {
	var (
		mergeRequestNeedRebase      []*gitlab.MergeRequest
		mergeRequestRunningPipeline []*gitlab.MergeRequest
	)

	mergeRequests := listProjectMergeRequests(project.ID)
	log.WithField("projectPath", project.Path).Infoln("merge request count", len(mergeRequests))

	branchMergeRequests := make(map[string][]*gitlab.MergeRequest)
	branchMergerIDs := make(map[string]MergerIDSet)
	for _, mr := range mergeRequests {
		branchMergeRequests[mr.TargetBranch] = append(branchMergeRequests[mr.TargetBranch], mr)
		branchMergerIDs[mr.TargetBranch] = listEligibleMergers(project.ID, mr.TargetBranch)
	}

	for _, mr := range mergeRequests {
		log := log.WithFields(log.Fields{
			"projectPath":  project.Path,
			"mergeRequest": mr.Title,
		})

		// because projectMergeRequest lacks some fields
		mergeRequest, _, err := client.MergeRequests.GetMergeRequest(project.ID, mr.IID, nil)
		if err != nil {
			log.WithField("mergeRequest", mr.IID).Errorln("failed to get merge request info")
		}

		log.Infof("processing `%s`\n", mergeRequest.Title)

		// the order of the following checks matters
		if !isMergeRequestReady(mergeRequest) {
			log.Debugln("merge request is not ready")
			continue
		}
		if !isMergeRequestApproved(mergeRequest, project.ID, branchMergerIDs[mr.TargetBranch]) {
			log.Debugln("merge request not be appproved")
			continue
		}
		if isMergeRequestPipelineFailed(mergeRequest) {
			log.Debugln("merge request does not success")
			continue
		}
		if isMergeRequestNeedRebase(mergeRequest, project.ID) {
			log.Debugln("merge request needs rebase")
			mergeRequestNeedRebase = append(mergeRequestNeedRebase, mergeRequest)
			continue
		}

		opts := gitlab.AcceptMergeRequestOptions{
			Squash:                    gitlab.Bool(true),
			MergeWhenPipelineSucceeds: gitlab.Bool(true),
		}

		if isMergeRequestPipelineRunning(mergeRequest) {
			log.Infoln("set to merge when pipeline become success")
			client.MergeRequests.AcceptMergeRequest(project.ID, mergeRequest.IID, &opts)
			mergeRequestRunningPipeline = append(mergeRequestRunningPipeline, mergeRequest)
			continue
		}

		log.Infoln("merging")
		client.MergeRequests.AcceptMergeRequest(project.ID, mergeRequest.IID, &opts)
		return
	}

	// rebase merge request only if there are no merge requests running pipeline
	// otherwise rebasing is a waste of runner resource
	if len(mergeRequestRunningPipeline) == 0 {
		// here we rebase all merge requests and see which will get merged firstly
		// shoud make a smarter algorithm
		for _, mergeRequest := range mergeRequestNeedRebase {
			if isMergeRequestNeedResolveConflicts(mergeRequest) {
				continue
			}
			log.WithFields(log.Fields{
				"projectID":    project.ID,
				"mergeRequest": mergeRequest.IID,
			}).Infoln("rebasing")
			client.MergeRequests.RebaseMergeRequest(project.ID, mergeRequest.IID)
		}
	}
}

func listEligibleMergers(projectID int, branch string) MergerIDSet {
	var mergerIDSet = make(MergerIDSet)

	for _, m := range listProjectMaintainers(projectID) {
		mergerIDSet[m.ID] = true
	}

	protectedBranch, resp, err := client.ProtectedBranches.GetProtectedBranch(projectID, branch)

	if err != nil {
		// not protected branch
		// TODO: improve
		if resp.StatusCode == 404 {
			for _, m := range append(listProjectMaintainers(projectID), listProjectDevelopers(projectID)...) {
				mergerIDSet[m.ID] = true
			}
			return mergerIDSet
		} else {
			log.WithFields(log.Fields{
				"projectID": projectID,
				"branch":    branch,
			}).Errorln("failed to get projected branch info", err)
		}
	}

	var branchAccessDescriptions []*gitlab.BranchAccessDescription
	branchAccessDescriptions = append(branchAccessDescriptions, protectedBranch.MergeAccessLevels...)
	branchAccessDescriptions = append(branchAccessDescriptions, protectedBranch.PushAccessLevels...)
	for _, mal := range branchAccessDescriptions {
		if mal.UserID != 0 {
			mergerIDSet[mal.UserID] = true
		} else {
			switch mal.AccessLevelDescription {
			case "Maintainers":
				{
					// maintainers have been added above
					continue
				}
			case "Developers + Maintainers":
				{
					for _, u := range listProjectDevelopers(projectID) {
						mergerIDSet[u.ID] = true
					}
				}
			case "No one":
				{
					continue
				}
			default:
				{
					log.WithFields(log.Fields{
						"projectID":              projectID,
						"branch":                 branch,
						"accessLevel":            mal.AccessLevel,
						"accessLevelDescription": mal.AccessLevelDescription,
					}).Warningln("unknown merge access level")
				}
			}
		}
	}

	return mergerIDSet
}

func listProjectMaintainers(projectID int) []*gitlab.ProjectMember {
	var (
		maintainers []*gitlab.ProjectMember
		options     = &gitlab.ListProjectMembersOptions{}
	)

	for {
		members, resp, err := client.ProjectMembers.ListAllProjectMembers(projectID, options)
		if err != nil {
			log.WithFields(log.Fields{"projectID": projectID}).Panicln("failed to get project members")
		}

		for _, member := range members {
			// owner: 50
			// maintainers: 40
			if member.AccessLevel >= 40 {
				maintainers = append(maintainers, member)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return maintainers
}

func listProjectDevelopers(projectID int) []*gitlab.ProjectMember {
	var (
		developers []*gitlab.ProjectMember
		options    = &gitlab.ListProjectMembersOptions{}
	)

	for {
		members, resp, err := client.ProjectMembers.ListAllProjectMembers(projectID, options)
		if err != nil {
			log.WithFields(log.Fields{"projectID": projectID}).Panicln("failed to get project members")
		}

		for _, member := range members {
			if member.AccessLevel == 30 {
				developers = append(developers, member)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return developers
}

func listProjectMergeRequests(projectID int) []*gitlab.MergeRequest {
	var (
		mergeRequests = []*gitlab.MergeRequest{}
		options       = &gitlab.ListProjectMergeRequestsOptions{
			State:   gitlab.String("opened"),
			Draft:   gitlab.Bool(false),
			WIP:     gitlab.String("no"),
			OrderBy: gitlab.String("created_at"),
			Sort:    gitlab.String("asc"),
		}
	)

	for {
		mrs, resp, err := client.MergeRequests.ListProjectMergeRequests(projectID, options)
		if err != nil {
			log.WithFields(log.Fields{"projectID": projectID}).Errorln("failed to get project merge requests")
		}

		mergeRequests = append(mergeRequests, mrs...)
		if resp.NextPage == 0 {
			break
		}
		options.Page = resp.NextPage
	}

	return mergeRequests
}

func isMergeRequestReady(mergeRequest *gitlab.MergeRequest) bool {
	return !mergeRequest.WorkInProgress && !mergeRequest.Draft
}

func isMergeRequestNeedResolveConflicts(mergeRequest *gitlab.MergeRequest) bool {
	return mergeRequest.HasConflicts
}

func isMergeRequestNeedRebase(mergeRequest *gitlab.MergeRequest, projectID int) bool {
	var sourceBranchBaseSha = mergeRequest.DiffRefs.BaseSha
	var targetBranchHeadSha = func() string {
		branch, _, err := client.Branches.GetBranch(projectID, mergeRequest.TargetBranch)
		if err != nil {
			log.WithFields(log.Fields{
				"projectID": projectID,
				"branch":    mergeRequest.TargetBranch,
			}).Errorln("failed to get branch info")
		}
		return branch.Commit.ID
	}()
	return sourceBranchBaseSha != targetBranchHeadSha
}

func isMergeRequestPipelineSucceed(mergeRequest *gitlab.MergeRequest) bool {
	status := mergeRequest.HeadPipeline.Status
	return status == "success" || status == "skipped"
}

func isMergeRequestPipelineFailed(mergeRequest *gitlab.MergeRequest) bool {
	status := mergeRequest.HeadPipeline.Status
	return status == "failed" || status == "cancelled"
}

func isMergeRequestPipelineRunning(mergeRequest *gitlab.MergeRequest) bool {
	return !isMergeRequestPipelineSucceed(mergeRequest) && !isMergeRequestPipelineFailed(mergeRequest)
}

func isMergeRequestApproved(mergeRequest *gitlab.MergeRequest, projectID int, mergers MergerIDSet) bool {
	mergeRequestApprovals, _, err := client.MergeRequestApprovals.GetApprovalState(projectID, mergeRequest.IID, nil)
	if err != nil {
		log.WithFields(log.Fields{
			"projectID":    projectID,
			"mergeRequest": mergeRequest.IID,
		}).Errorln("failed to get merge request approval info")
	}

	for _, approvalRule := range mergeRequestApprovals.Rules {
		for _, approver := range approvalRule.ApprovedBy {
			if _, ok := mergers[approver.ID]; ok {
				return true
			}
		}
	}
	return false
}

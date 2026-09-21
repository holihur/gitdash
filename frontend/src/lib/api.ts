// 会话通过 httpOnly Cookie(gitdash_session) 自动携带，前端不再持有 token。

export * from "./api/core";
export * from "./api/types";

import { authApi } from "./api/auth";
import { announcementApi } from "./api/announcement";
import { reposApi } from "./api/repos";
import { inboxApi } from "./api/inbox";
import { issuesApi } from "./api/issues";
import { orgsApi } from "./api/orgs";
import { webhooksApi } from "./api/webhooks";
import { pipelineApi } from "./api/pipeline";
import { searchApi } from "./api/search";
import { releasesApi } from "./api/releases";
import { keysApi } from "./api/keys";
import { packagesApi } from "./api/packages";
import { projectsApi } from "./api/projects";
import { usersApi } from "./api/users";
import { copilotApi } from "./api/copilot";
import { applicationsApi } from "./api/applications";
import { feedbackApi } from "./api/feedback";
import { connectionsApi } from "./api/connections";

export const api = {
  ...authApi,
  ...announcementApi,
  ...reposApi,
  ...inboxApi,
  ...issuesApi,
  ...orgsApi,
  ...webhooksApi,
  ...pipelineApi,
  ...searchApi,
  ...releasesApi,
  ...keysApi,
  ...packagesApi,
  ...projectsApi,
  ...usersApi,
  ...copilotApi,
  ...applicationsApi,
  ...feedbackApi,
  ...connectionsApi,
};

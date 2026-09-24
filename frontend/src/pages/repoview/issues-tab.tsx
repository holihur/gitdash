import RepoIssues from "@/pages/RepoIssues";
import IssueDetail from "./issue-detail";

export default function IssuesTab({
  owner,
  name,
  role,
  issueNumber,
}: {
  owner: string;
  name: string;
  role?: string;
  issueNumber: number;
}) {
  if (issueNumber > 0) {
    return <IssueDetail owner={owner} name={name} role={role} number={issueNumber} />;
  }
  return <RepoIssues owner={owner} name={name} role={role} />;
}

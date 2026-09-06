import RepoIssues from "@/pages/RepoIssues";

export default function IssuesTab({ owner, name }: { owner: string; name: string }) {
  return <RepoIssues owner={owner} name={name} />;
}

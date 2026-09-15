import RepoIssues from "@/pages/RepoIssues";

export default function IssuesTab({
  owner,
  name,
  role,
}: {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}) {
  return <RepoIssues owner={owner} name={name} role={role} />;
}

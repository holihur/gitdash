import RepoPulls from "@/pages/RepoPulls";

export interface PullsTabProps {
  owner: string;
  name: string;
  role?: "owner" | "read" | "write";
}

export default function PullsTab({ owner, name, role }: PullsTabProps) {
  return <RepoPulls owner={owner} name={name} role={role} />;
}

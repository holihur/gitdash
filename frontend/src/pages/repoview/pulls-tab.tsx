import RepoPulls from "@/pages/RepoPulls";

export default function PullsTab({ owner, name }: { owner: string; name: string }) {
  return <RepoPulls owner={owner} name={name} />;
}

import RepoPipeline from "@/pages/RepoPipeline";

export interface PipelineTabProps {
  owner: string;
  name: string;
  role?: string;
}

export default function PipelineTab({ owner, name, role }: PipelineTabProps) {
  return <RepoPipeline owner={owner} name={name} role={role} />;
}

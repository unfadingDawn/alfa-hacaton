import { ResearchView } from "@/features/news/research";

export default async function ResearchPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ResearchView id={id} />;
}

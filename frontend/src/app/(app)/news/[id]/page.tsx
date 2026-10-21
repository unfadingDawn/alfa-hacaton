import { ArticleView } from "@/features/news/analysis";

export default async function NewsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return <ArticleView id={id} />;
}

// Типы ответов API (соответствуют backend/api/openapi.yaml).

export type Role = { code: string; title: string; focus: string[]; topics: string[] };
export type Topic = { code: string; name: string; description: string };

export type Personalization = {
  min_relevance: number;
  include_research: boolean;
  horizon: "short" | "medium" | "long";
  detail: "brief" | "detailed";
};

export type Me = {
  id: string;
  email: string;
  full_name: string;
  role_code: string;
  role?: Role;
  position_title: string;
  work_focus: string;
  personalization: Personalization;
  onboarding_completed: boolean;
  avatar?: string | null;
  interests: { topic_code: string; weight: number; name: string }[];
  companies: {
    id: string;
    display_name: string | null;
    okved_code: string;
    okved_name: string;
    region: string | null;
    revenue: number | null;
    is_primary: boolean;
  }[];
};

export type Fact = { id: string; label: string; value: string; source: string };
export type Exposure = { code: string; title: string; explanation: string; topics: string[]; fact_ids: string[] };
export type Counterparty = {
  id: string;
  amount: number;
  pay_count: number;
  okved_code?: string;
  okved_name?: string;
  region?: string;
  known: boolean;
};
export type Industry = {
  direction: "outgoing" | "incoming";
  division: string;
  sample_name: string;
  counterparties: number;
  amount: number;
  share: number;
};
export type Application = { product: string; amount?: number | null; status?: string };

// Сведения с главного экрана личного кабинета «Альфа-Бизнес» (все необязательные).
export type BusinessProfile = {
  source?: "user" | "alfa_business";
  updated_at?: string;
  legal_name?: string;
  manager_name?: string;
  companies_in_account?: number;
  accounts_rub?: number;
  accounts_fx?: number;
  currencies?: string[];
  balance_rub?: number | null;
  overdraft_available?: boolean | null;
  balance_with_overdraft_rub?: number | null;
  business_cards?: boolean | null;
  tariff?: string;
  risk_indicator?: "" | "low" | "medium" | "high";
  gov_procurement?: boolean | null;
  applications?: Application[];
};

export type UserCompanyProfile = {
  description?: string;
  products?: string;
  markets?: string;
  suppliers?: string;
  priorities?: string[];
  notes?: string;
  business?: BusinessProfile;
};
export type CompanyProfile = {
  id: string;
  display_name: string;
  section: { letter: string; name: string };
  okved_code: string;
  okved_name: string;
  region: string;
  city: string;
  revenue: number | null;
  group_revenue?: number | null;
  group_size?: number;
  size_segment: string;
  is_large: boolean;
  facts: Fact[];
  exposures: Exposure[];
  topic_hints: string[];
  fts_countries: string[];
  user_profile: UserCompanyProfile;
  relations?: {
    outgoing_amount: number;
    outgoing_count: number;
    supplier_count: number;
    incoming_amount: number;
    incoming_count: number;
    customer_count: number;
    top_recipients: Counterparty[];
    top_senders: Counterparty[];
    industries: Industry[];
    group_id?: string;
    group_size: number;
    group_members: Counterparty[];
    linked_persons: number;
    person_regions: string[];
    companies_sharing_persons: Counterparty[];
  };
  data_gaps: string[];
};

export type CompanySearchItem = {
  id: string;
  display_name: string | null;
  okved_code: string;
  okved_name: string;
  region: string | null;
  city: string | null;
  revenue: number | null;
  group_revenue: number | null;
  group_size: number | null;
  size_segment: string;
  is_large: boolean;
  client_type: string | null;
  eco_stage: string | null;
  workers_fns: string | null;
  section: { letter: string; name: string };
};

export type Importance = "low" | "medium" | "high" | "critical";
export type Impact = "risk" | "opportunity" | "mixed" | "neutral";

export type AnalysisRef = {
  method: "llm" | "rules";
  depth: "brief" | "full";
  relevance_score: number;
  importance_level: Importance;
  impact_type: Impact;
  matched_topics: string[];
  why_it_matters: string;
};

export type FeedItem = {
  id: string;
  url: string;
  title: string;
  summary: string;
  published_at: string;
  source: { code: string; name: string; kind: "rss" | "alfa_research" | "user_submitted" };
  topics: { code: string; name: string; method: "llm" | "keywords" | "metadata" }[];
  analysis: AnalysisRef | null;
  is_saved: boolean;
};

export type Evidence = {
  id: string;
  type: "article" | "related_news" | "bank_research" | "company_data";
  title: string;
  url?: string;
  date?: string;
  excerpt: string;
  page?: number;
  note?: string;
};

export type Claim = { text: string; source_ids: string[] };
export type RiskOpp = {
  title: string;
  description: string;
  likelihood: "low" | "medium" | "high";
  horizon: "short" | "medium" | "long";
  basis: "fact" | "bank_view" | "model_inference";
  source_ids: string[];
};

export type Insight = {
  relevance_score: number;
  importance_level: Importance;
  impact_type: Impact;
  matched_topics: string[];
  what_happened: { summary: string; facts: Claim[] };
  why_it_matters: string;
  why_it_matters_source_ids: string[];
  potential_impact: string;
  risks: RiskOpp[];
  opportunities: RiskOpp[];
  recommended_actions: {
    action: string;
    object?: string;
    owner: string;
    horizon?: "short" | "medium" | "long";
    rationale: string;
    source_ids: string[];
  }[];
  used_company_factors?: { fact_id: string; mechanism: string }[];
  bank_view: Claim[];
  assumptions: string[];
  uncertainty_notes: string[];
  evidence_sources: Evidence[];
  validation_notes?: string[];
  fact_check?: FactCheck;
  source_coverage?: { confirmed: number; total: number; ratio: number };
  source_completeness?: SourceCompleteness;
};

export type ContentLevel = "title_only" | "summary" | "full_text";
export type SourceCompleteness = { level: ContentLevel; label: string; note?: string };
export type UnverifiedValue = {
  path: string;
  kind: "percent" | "money" | "term" | "date" | "year" | "entity";
  value: string;
  claim: string;
  action: "removed" | "marked";
};
export type FactCheck = { checked: number; verified: number; unverified: UnverifiedValue[] };

export type RunStep = {
  name: string;
  title: string;
  model?: string;
  status: "ok" | "error";
  duration_ms: number;
  tokens: number;
  cached?: boolean;
  summary: string;
};

export type Run = {
  id: string;
  status: "running" | "succeeded" | "failed";
  models: string[];
  prompt_tokens: number;
  completion_tokens: number;
  steps: RunStep[] | null;
  error: string | null;
  started_at: string;
  finished_at: string | null;
};

export type FullAnalysis = {
  id: string;
  relevance_score: number;
  importance_level: Importance;
  impact_type: Impact;
  matched_topics: string[];
  why_it_matters: string;
  role_code: string;
  created_at: string;
  result: { insight: Insight; role?: Role; models?: string[]; generated_at?: string } | null;
};

export type ArticleDetail = {
  id: string;
  url: string;
  title: string;
  summary: string;
  body?: string;
  published_at: string;
  source: FeedItem["source"];
  topics: FeedItem["topics"];
  brief?: AnalysisRef;
  analysis?: FullAnalysis;
  run?: Run;
  is_saved: boolean;
  feedback?: { rating: number; comment: string };
  content_level?: ContentLevel | null;
  updated_at?: string | null;
  source_completeness?: SourceCompleteness;
};

export type GuardStatus = {
  day: string;
  calls: number;
  daily_call_limit: number;
  tokens_used: number;
  daily_token_budget: number;
  paused_until?: string;
  pause_reason?: string;
};

export type LlmBrief = { configured: boolean; host: string; guard: GuardStatus };

export type Dashboard = {
  top_important: FeedItem[];
  company_related: FeedItem[];
  market: { bank_research: FeedItem[]; regulator: FeedItem[] };
  risks: { article_id: string; article_title: string; title: string; description: string; importance_level: Importance }[];
  opportunities: Dashboard["risks"];
  risks_note: string;
  day_summary: {
    text: string;
    stats: { new_articles: number; high_importance: number; high_importance_llm: number; full_analyses: number };
    topics: { code: string; name: string; articles: number }[];
  };
  llm: LlmBrief;
};

// Уточняющие вопросы к готовому анализу.
export type AnswerPoint = {
  text: string;
  source_ids: string[];
  basis: "fact" | "bank_view" | "company_data" | "model_inference";
};

export type QAAnswer = {
  answer: string;
  points: AnswerPoint[];
  not_in_sources: string;
  out_of_scope: boolean;
  sources: Evidence[];
  validation_notes?: string[];
};

export type AnalysisQuestion = {
  id: string;
  question: string;
  preset: string | null;
  method: "llm" | "deterministic";
  answer: QAAnswer;
  source_ids: string[];
  model: string;
  prompt_version: string;
  tokens: number;
  duration_ms: number;
  cached: boolean;
  created_at: string;
  outdated: boolean;
};

export type QuestionLimits = { per_analysis: number; used_for_analysis: number; per_user_daily: number; used_today: number };
export type QuickQuestion = { code: string; text: string; deterministic: boolean };

export type QuestionHistory = {
  analysis_id: string;
  items: AnalysisQuestion[];
  limits: QuestionLimits;
  quick_questions: QuickQuestion[];
};

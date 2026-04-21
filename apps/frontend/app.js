import React, { useEffect, useMemo, useState } from "https://esm.sh/react@18.2.0";
import { createRoot } from "https://esm.sh/react-dom@18.2.0/client";

const h = React.createElement;

const navItems = [
  { id: "home", label: "Home", icon: "H" },
  { id: "research", label: "Research", icon: "R" },
  { id: "create", label: "Create", icon: "C" },
  { id: "track", label: "Track", icon: "T" },
  { id: "library", label: "Library", icon: "L" },
  { id: "automations", label: "Automations", icon: "A" },
  { id: "system", label: "System", icon: "S" },
];

const pageRoutes = {
  home: "/",
  research: "/research",
  create: "/create",
  track: "/track",
  library: "/library",
  automations: "/automations/workflows",
  system: "/system/runs",
};

function pageFromHash() {
  const path = window.location.hash.replace(/^#/, "") || "/";
  if (path.startsWith("/research")) return "research";
  if (path.startsWith("/create")) return "create";
  if (path.startsWith("/track")) return "track";
  if (path.startsWith("/library")) return "library";
  if (path.startsWith("/automations")) return "automations";
  if (path.startsWith("/system")) return "system";
  return "home";
}

const workspaceTemplates = {
  default: {
    title: "Generic Workflow",
    inputFields: [
      ["Input payload", "{ }"],
      ["Preset", "Manual run"],
      ["Review policy", "Workflow default"],
    ],
    steps: ["Validate inputs", "Execute nodes", "Collect outputs", "Apply review policy"],
    artifacts: ["result.json", "run_events.json"],
    preview:
      "Generic workflow output\n\nThis fallback workspace keeps execution controls, event observability, and artifact handling available for any workflow type.",
    exports: ["JSON", "Run trace"],
  },
  "paper-search": {
    title: "Paper Search + Summary",
    workspaceType: "paper",
    inputFields: [
      ["Research query", "agentic coding papers"],
      ["Sources", "arXiv, Semantic Scholar, uploaded PDFs"],
      ["Date range", "Last 5 years"],
      ["Summary depth", "Detailed"],
      ["Citation style", "BibTeX"],
    ],
    steps: ["Search papers", "Rank results", "Fetch abstracts", "Summarize findings", "Await review"],
    artifacts: ["paper_results.json", "summary.md", "citations.bib"],
    preview:
      "Summary draft\n\nRecent work clusters around tool use, planning, benchmark design, and code-editing agents. Highest-confidence citations are attached in citations.bib.",
    exports: ["Markdown", "PDF", "BibTeX"],
  },
  "video-generator": {
    title: "Video Generation",
    workspaceType: "video",
    inputFields: [
      ["Source content", "Launch notes and outline"],
      ["Style", "Explainer"],
      ["Length", "90 seconds"],
      ["Voice", "Neutral"],
      ["Aspect ratio", "16:9"],
    ],
    steps: ["Generate outline", "Draft script", "Create storyboard", "Render preview", "Await review"],
    artifacts: ["script.md", "storyboard.json", "captions.srt", "video.mp4"],
    preview:
      "Scene 1: introduce the workflow goal.\nScene 2: show input assets.\nScene 3: explain review gate before final export.",
    exports: ["MP4", "SRT", "Storyboard JSON"],
  },
  "meal-logger": {
    title: "Meal Logging + Nutrition",
    workspaceType: "default",
    inputFields: [
      ["Meal description", "Chicken rice bowl with avocado"],
      ["Portion hints", "Large bowl"],
      ["Meal time", "Lunch"],
      ["Goals", "Protein, fiber"],
      ["Review policy", "Only for medical-risk advice"],
    ],
    steps: ["Parse meal", "Estimate portions", "Analyze nutrition", "Generate notes", "Flag uncertainty"],
    artifacts: ["meal_log.json", "nutrition_summary.md", "daily_totals.csv"],
    preview:
      "Estimated nutrition\n\nProtein: 42g\nCarbs: 58g\nFat: 24g\nConfidence: medium due to portion uncertainty.",
    exports: ["CSV", "JSON", "Markdown"],
  },
};

const sampleWorkflows = [
  {
    id: "paper-search",
    name: "Paper Search + Summary",
    kind: "ai_workflow",
    description: "Find papers, rank sources, and produce reviewed summaries.",
    node_count: 5,
    active_version: 1,
    status: "ready",
    ai: {
      purpose: "Research discovery and synthesis",
      models: ["gpt-5-mini", "text-embedding"],
      risk_level: "medium",
      human_review_required: true,
      workspaceType: "paper",
    },
  },
  {
    id: "video-generator",
    name: "Video Generator",
    kind: "ai_workflow",
    description: "Generate scripts, storyboards, captions, and video artifacts.",
    node_count: 6,
    active_version: 1,
    status: "paused",
    ai: {
      purpose: "Video production from user-provided content",
      models: ["gpt-5-mini", "video-renderer"],
      risk_level: "high",
      human_review_required: true,
      workspaceType: "video",
    },
  },
  {
    id: "meal-logger",
    name: "Meal Logger",
    kind: "ai_workflow",
    description: "Log meals, estimate nutrition, and track daily history.",
    node_count: 4,
    active_version: 1,
    status: "completed",
    ai: {
      purpose: "Personal nutrition analysis",
      models: ["gpt-5-mini"],
      risk_level: "low",
      human_review_required: false,
      workspaceType: "default",
    },
  },
];

const sampleRuns = [
  {
    id: "run-paper-001",
    workflow_id: "paper-search",
    workflow_name: "Paper Search + Summary",
    workflow_kind: "ai_workflow",
    status: "completed",
    started_at: "2026-04-21T00:03:00Z",
    finished_at: "2026-04-21T00:03:38Z",
    duration_ms: 38000,
    ai: sampleWorkflows[0].ai,
    input: { query: [{ text: "agentic coding papers" }] },
    result: { summary: [{ file: "summary.md" }] },
    events: [
      { type: "run.started", occurred_at: "2026-04-21T00:03:00Z", node_id: "start" },
      { type: "node.completed", occurred_at: "2026-04-21T00:03:08Z", node_id: "search" },
      {
        type: "ai_model_call",
        occurred_at: "2026-04-21T00:03:20Z",
        node_id: "summarize",
        fields: {
          provider: "openai",
          model: "gpt-5-mini",
          prompt_hash: "ph_37af91c8",
          prompt_preview: "Summarize the ranked papers and identify claims that need citation.",
          input_tokens: 1850,
          output_tokens: 740,
          total_tokens: 2590,
          latency_ms: 6100,
          status: "ok",
        },
      },
      { type: "artifact.created", occurred_at: "2026-04-21T00:03:33Z", node_id: "write" },
      { type: "run.completed", occurred_at: "2026-04-21T00:03:38Z", node_id: "finish" },
    ],
  },
  {
    id: "run-video-001",
    workflow_id: "video-generator",
    workflow_name: "Video Generator",
    workflow_kind: "ai_workflow",
    status: "paused",
    started_at: "2026-04-21T00:04:00Z",
    duration_ms: 51000,
    ai: sampleWorkflows[1].ai,
    events: [
      { type: "run.started", occurred_at: "2026-04-21T00:04:00Z", node_id: "start" },
      { type: "ai_model_call", occurred_at: "2026-04-21T00:04:19Z", node_id: "script", fields: { model: "gpt-5-mini", prompt_hash: "ph_98aa201e", total_tokens: 3210, latency_ms: 9200, status: "ok" } },
      { type: "review.created", occurred_at: "2026-04-21T00:04:51Z", node_id: "review-script" },
    ],
  },
];

const sampleReviews = [
  {
    id: "review-video-001",
    run_id: "run-video-001",
    workflow_id: "video-generator",
    workflow_name: "Video Generator",
    workflow_kind: "ai_workflow",
    node_id: "review-script",
    node_name: "Review Script",
    status: "pending",
    created_at: "2026-04-21T00:04:51Z",
    proposed_output:
      "Draft script: introduce the product, explain the workflow workspace, show review controls, and end with export options.",
    context: {
      risk_level: "high",
      model: "gpt-5-mini",
      prompt_hash: "ph_98aa201e",
    },
  },
  {
    id: "review-paper-001",
    run_id: "run-paper-001",
    workflow_id: "paper-search",
    workflow_name: "Paper Search + Summary",
    workflow_kind: "ai_workflow",
    node_id: "claim-check",
    node_name: "Claim Review",
    status: "pending",
    created_at: "2026-04-21T00:03:34Z",
    proposed_output: "The summary cites three high-confidence papers and flags two uncited claims for revision.",
    context: {
      risk_level: "medium",
      model: "gpt-5-mini",
      prompt_hash: "ph_37af91c8",
    },
  },
];

const sampleAssets = [
  { id: "asset-summary", name: "summary.md", type: "Markdown", workflow_id: "paper-search", workflow_name: "Paper Search + Summary", run_id: "run-paper-001", review_state: "approved", created_at: "2026-04-21T00:03:33Z" },
  { id: "asset-citations", name: "citations.bib", type: "BibTeX", workflow_id: "paper-search", workflow_name: "Paper Search + Summary", run_id: "run-paper-001", review_state: "approved", created_at: "2026-04-21T00:03:33Z" },
  { id: "asset-video", name: "video.mp4", type: "Video", workflow_id: "video-generator", workflow_name: "Video Generator", run_id: "run-video-001", review_state: "pending", created_at: "2026-04-21T00:05:00Z" },
  { id: "asset-meals", name: "daily_totals.csv", type: "CSV", workflow_id: "meal-logger", workflow_name: "Meal Logger", run_id: "run-meal-001", review_state: "not_required", created_at: "2026-04-21T00:12:00Z" },
];

async function fetchData(path, options) {
  const response = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (!response.ok) {
    throw new Error(`${path} returned ${response.status}`);
  }
  const body = await response.json();
  return body.data || body;
}

function useRivuletData() {
  const [loading, setLoading] = useState(true);
  const [warnings, setWarnings] = useState([]);
  const [metrics, setMetrics] = useState(null);
  const [workflows, setWorkflows] = useState(sampleWorkflows);
  const [runs, setRuns] = useState(sampleRuns);
  const [reviews, setReviews] = useState(sampleReviews);

  async function load() {
    setLoading(true);
    const nextWarnings = [];
    const [metricsResult, workflowResult, workflowFileResult, runResult, reviewResult] = await Promise.allSettled([
      fetchData("/dashboard/metrics"),
      fetchData("/workflows"),
      fetchData("/workflows/files"),
      fetchData("/runs?limit=40"),
      fetchData("/reviews"),
    ]);

    if (metricsResult.status === "fulfilled") {
      setMetrics(metricsResult.value.metrics || metricsResult.value);
    } else {
      nextWarnings.push("Dashboard metrics are unavailable; showing local sample counts.");
    }

    const persisted = workflowResult.status === "fulfilled" ? workflowResult.value.workflows || [] : [];
    const files = workflowFileResult.status === "fulfilled" ? workflowFileResult.value.workflows || [] : [];
    const nextWorkflows = [...persisted, ...files].map(normalizeWorkflow);
    setWorkflows(nextWorkflows.length > 0 ? nextWorkflows : sampleWorkflows);
    if (workflowResult.status === "rejected" && workflowFileResult.status === "rejected") {
      nextWarnings.push("Workflow APIs are unavailable; showing sample workflows.");
    }

    const nextRuns = runResult.status === "fulfilled" ? (runResult.value.runs || []).map(normalizeRun) : [];
    setRuns(nextRuns.length > 0 ? nextRuns : sampleRuns);
    if (runResult.status === "rejected") {
      nextWarnings.push("Run history is unavailable; showing sample runs.");
    }

    const nextReviews = reviewResult.status === "fulfilled" ? (reviewResult.value.reviews || []).map(normalizeReview) : [];
    setReviews(nextReviews.length > 0 ? nextReviews : sampleReviews);
    if (reviewResult.status === "rejected") {
      nextWarnings.push("Review queue is unavailable; showing sample review items.");
    }

    setWarnings(nextWarnings);
    setLoading(false);
  }

  useEffect(() => {
    load();
  }, []);

  return { loading, warnings, metrics, workflows, runs, reviews, setReviews, reload: load };
}

function normalizeWorkflow(workflow, index) {
  const ai = workflow.ai || {};
  const name = workflow.name || workflow.file_name || `Workflow ${index + 1}`;
  const workspaceType = normalizeWorkspaceType(workflow.workspaceType || workflow.workspace_type || ai.workspaceType || ai.workspace_type || inferWorkspaceType(name));
  return {
    id: workflow.id || workflow.workflow_id || slugify(name),
    name,
    kind: workflow.kind || (workflow.ai ? "ai_workflow" : "automation"),
    description: workflow.description || ai.purpose || "Saved workflow",
    node_count: workflow.node_count || 0,
    active_version: workflow.active_version || 1,
    status: workflow.status || "ready",
    ai: {
      purpose: ai.purpose || workflow.description || "Workflow automation",
      models: ai.models || [],
      risk_level: ai.risk_level || "unknown",
      human_review_required: Boolean(ai.human_review_required),
      workspaceType,
    },
    sample_data: workflow.sample_data,
    path: workflow.path,
  };
}

function normalizeWorkspaceType(value) {
  const normalized = String(value || "default").toLowerCase().trim();
  return ["paper", "video"].includes(normalized) ? normalized : "default";
}

function inferWorkspaceType(name) {
  const normalized = String(name || "").toLowerCase();
  if (normalized.includes("paper") || normalized.includes("research") || normalized.includes("summary")) return "paper";
  if (normalized.includes("video") || normalized.includes("storyboard") || normalized.includes("caption")) return "video";
  return "default";
}

function normalizeRun(run) {
  return {
    ...run,
    workflow_name: run.workflow_name || run.workflow_id || "Workflow",
    status: run.status || "unknown",
    events: Array.isArray(run.events) ? run.events : [],
  };
}

function normalizeReview(review) {
  return {
    ...review,
    workflow_name: review.workflow_name || String(review.workflow_id || "Workflow"),
    status: review.status || "pending",
  };
}

function slugify(value) {
  return String(value || "item")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/(^-|-$)/g, "");
}

function formatDate(value) {
  if (!value) return "Not recorded";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return date.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function formatDuration(ms) {
  if (!ms) return "0s";
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${Math.round(ms / 1000)}s`;
  return `${Math.round(ms / 60000)}m`;
}

function statusTone(status) {
  const normalized = String(status || "").toLowerCase();
  if (["completed", "approved", "ok", "success", "ready"].includes(normalized)) return "green";
  if (["running", "in_progress"].includes(normalized)) return "blue";
  if (["paused", "pending", "needs_review", "review"].includes(normalized)) return "amber";
  if (["failed", "rejected", "cancelled", "error"].includes(normalized)) return "red";
  return "gray";
}

function riskTone(risk) {
  const normalized = String(risk || "").toLowerCase();
  if (normalized === "low") return "green";
  if (normalized === "medium") return "amber";
  if (normalized === "high") return "red";
  return "gray";
}

function workflowTemplate(workflow) {
  if (!workflow) return workspaceTemplates.default;
  if (workspaceTemplates[workflow.id]) return workspaceTemplates[workflow.id];
  if (workflow.ai?.workspaceType === "paper") return workspaceTemplates["paper-search"];
  if (workflow.ai?.workspaceType === "video") return workspaceTemplates["video-generator"];
  if (workflow.ai?.workspaceType === "default") return workspaceTemplates.default;
  const name = workflow.name.toLowerCase();
  if (name.includes("video")) return workspaceTemplates["video-generator"];
  if (name.includes("meal") || name.includes("nutrition")) return workspaceTemplates["meal-logger"];
  return workspaceTemplates.default;
}

function workflowByType(workflows, workspaceType) {
  return workflows.find((workflow) => normalizeWorkspaceType(workflow.ai?.workspaceType) === workspaceType);
}

function workflowModels(workflow) {
  const models = workflow?.ai?.models || [];
  return models.length > 0 ? models.join(", ") : "Not declared";
}

function deriveAssets(runs, workflows) {
  const derived = [];
  for (const run of runs) {
    const workflow = workflows.find((item) => item.id === run.workflow_id);
    const result = run.result || {};
    for (const [nodeID, items] of Object.entries(result)) {
      if (!Array.isArray(items)) continue;
      items.forEach((item, index) => {
        const file = item.file || item.path || item.filename || item.name;
        if (!file) return;
        derived.push({
          id: `${run.id}-${nodeID}-${index}`,
          name: String(file),
          type: guessAssetType(String(file)),
          workflow_id: run.workflow_id,
          workflow_name: run.workflow_name || workflow?.name || "Workflow",
          run_id: run.id,
          review_state: run.status === "paused" ? "pending" : "not_required",
          created_at: run.finished_at || run.started_at,
        });
      });
    }
  }
  return derived.length > 0 ? derived : sampleAssets;
}

function guessAssetType(name) {
  const lower = name.toLowerCase();
  if (lower.endsWith(".md")) return "Markdown";
  if (lower.endsWith(".json")) return "JSON";
  if (lower.endsWith(".csv")) return "CSV";
  if (lower.endsWith(".mp4")) return "Video";
  if (lower.endsWith(".bib")) return "BibTeX";
  return "File";
}

function App() {
  const data = useRivuletData();
  const [page, setPage] = useState(pageFromHash);
  const [query, setQuery] = useState("");
  const [selectedWorkflowID, setSelectedWorkflowID] = useState("paper-search");
  const [selectedRunID, setSelectedRunID] = useState("run-paper-001");
  const [selectedReviewID, setSelectedReviewID] = useState("review-video-001");
  const [automationView, setAutomationView] = useState("catalog");
  const [systemView, setSystemView] = useState("runs");

  const workflows = data.workflows;
  const runs = data.runs;
  const reviews = data.reviews;
  const assets = useMemo(() => deriveAssets(runs, workflows), [runs, workflows]);
  const selectedWorkflow = workflows.find((item) => item.id === selectedWorkflowID) || workflows[0];
  const selectedRun = runs.find((item) => item.id === selectedRunID) || runs[0];
  const selectedReview = reviews.find((item) => item.id === selectedReviewID) || reviews[0];
  const pendingReviews = reviews.filter((item) => item.status === "pending").length;

  useEffect(() => {
    const syncPage = () => setPage(pageFromHash());
    window.addEventListener("hashchange", syncPage);
    return () => window.removeEventListener("hashchange", syncPage);
  }, []);

  function navigate(nextPage) {
    setPage(nextPage);
    const route = pageRoutes[nextPage] || "/";
    if (window.location.hash !== `#${route}`) {
      window.location.hash = route;
    }
  }

  function pageForWorkflow(workflow) {
    const type = normalizeWorkspaceType(workflow?.ai?.workspaceType);
    if (type === "paper") return "research";
    if (type === "video") return "create";
    if (workflow?.id === "meal-logger" || workflow?.name?.toLowerCase().includes("meal")) return "track";
    return "automations";
  }

  function openDomainWorkflow(id) {
    setSelectedWorkflowID(id);
    const latestRun = runs.find((run) => run.workflow_id === id);
    if (latestRun) setSelectedRunID(latestRun.id);
    const workflow = workflows.find((item) => item.id === id);
    navigate(pageForWorkflow(workflow));
  }

  function openAutomationWorkflow(id) {
    setSelectedWorkflowID(id);
    const latestRun = runs.find((run) => run.workflow_id === id);
    if (latestRun) setSelectedRunID(latestRun.id);
    setAutomationView("workspace");
    navigate("automations");
  }

  function openRun(id) {
    setSelectedRunID(id);
    setSystemView("runs");
    navigate("system");
  }

  function openReview(id) {
    setSelectedReviewID(id);
    setSystemView("reviews");
    navigate("system");
  }

  const pageNode = {
    home: h(HomePage, { data, workflows, runs, reviews, assets, openWorkflow: openDomainWorkflow, openRun, openReview, setPage: navigate }),
    research: h(ResearchPage, { workflows, runs, openRun }),
    create: h(CreatePage, { workflows, runs, openRun }),
    track: h(TrackPage, { workflows, runs, assets }),
    library: h(AssetsPage, { assets, workflows }),
    automations: h(AutomationsPage, { workflows, query, setQuery, selectedWorkflow, runs, openWorkflow: openAutomationWorkflow, automationView, setAutomationView, openRun }),
    system: h(SystemPage, { workflows, runs, selectedRun, setSelectedRunID, reviews, selectedReview, setSelectedReviewID, setReviews: data.setReviews, reload: data.reload, systemView, setSystemView }),
  }[page];

  return h(
    "div",
    { className: "app-shell" },
    h(Sidebar, { page, setPage: navigate, pendingReviews }),
    h(
      "main",
      { className: "main" },
      h(Topbar, { workflows, selectedWorkflow, query, setQuery, onWorkflowChange: openDomainWorkflow, reload: data.reload }),
      h(
        "div",
        { className: "content" },
        data.loading ? h("div", { className: "loading-strip" }, "Loading live Rivulet data...") : null,
        data.warnings.map((warning) => h("div", { className: "loading-strip", key: warning }, warning)),
        pageNode,
      ),
    ),
  );
}

function Sidebar({ page, setPage, pendingReviews }) {
  return h(
    "aside",
    { className: "sidebar" },
    h("div", { className: "brand" }, h("div", { className: "brand-title" }, "Rivulet"), h(Badge, { tone: "blue" }, "Console")),
    h(
      "nav",
      { className: "nav" },
      navItems.map((item) =>
        h(
          "button",
          {
            key: item.id,
            className: `nav-button ${page === item.id ? "active" : ""}`,
            onClick: () => setPage(item.id),
          },
          h("span", { className: "nav-label" }, h("span", { className: "nav-icon" }, item.icon), item.label),
          item.id === "system" && pendingReviews > 0 ? h("span", { className: "nav-count" }, pendingReviews) : null,
        ),
      ),
    ),
    h("div", { className: "sidebar-note" }, "Research, creation, and tracking are the primary surfaces. Workflows and traces live under Automations and System."),
  );
}

function Topbar({ workflows, selectedWorkflow, query, setQuery, onWorkflowChange, reload }) {
  return h(
    "div",
    { className: "topbar" },
    h(
      "div",
      { className: "topbar-left" },
      h("input", {
        className: "search",
        value: query,
        onChange: (event) => setQuery(event.target.value),
        placeholder: "Search tasks, outputs, automations, reviews",
      }),
    ),
    h(
      "div",
      { className: "topbar-right" },
      h(
        "select",
        {
          className: "select",
          value: selectedWorkflow?.id || "",
          onChange: (event) => onWorkflowChange(event.target.value),
          "aria-label": "Quick launch task",
        },
        workflows.map((workflow) => h("option", { key: workflow.id, value: workflow.id }, workflow.name)),
      ),
      h("button", { className: "button", onClick: reload }, "Refresh"),
    ),
  );
}

function PageHeader({ eyebrow, title, description, actions }) {
  return h(
    "section",
    { className: "page-header" },
    h("div", null, h("p", { className: "eyebrow" }, eyebrow), h("h1", null, title), description ? h("p", { className: "subtle" }, description) : null),
    actions ? h("div", { className: "button-row" }, actions) : null,
  );
}

function HomePage({ data, workflows, runs, reviews, assets, openWorkflow, openRun, openReview, setPage }) {
  const completed = runs.filter((run) => run.status === "completed").length;
  const failed = runs.filter((run) => run.status === "failed").length + Number(data.metrics?.failed_executions || 0);
  const active = runs.filter((run) => ["running", "paused"].includes(run.status)).length + Number(data.metrics?.instances || 0);
  const pending = reviews.filter((review) => review.status === "pending").length;
  const activity = [
    ...runs.slice(0, 3).map((run) => ({ id: run.id, title: `${run.workflow_name} ${run.status}`, at: run.started_at, action: () => openRun(run.id) })),
    ...reviews.slice(0, 2).map((review) => ({ id: review.id, title: `${review.workflow_name} review ${review.status}`, at: review.created_at, action: () => openReview(review.id) })),
  ];

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Home",
      title: "AI product workspace",
      description: "Start research, create media, track personal data, review approvals, and return to recent outputs.",
      actions: [
        h("button", { className: "button primary", key: "research", onClick: () => setPage("research") }, "Search Papers"),
        h("button", { className: "button", key: "video", onClick: () => setPage("create") }, "Generate Video"),
        h("button", { className: "button", key: "meal", onClick: () => setPage("track") }, "Log Meal"),
      ],
    }),
    h(
      "section",
      { className: "grid metrics-grid" },
      h(StatTile, { label: "Active Runs", value: active }),
      h(StatTile, { label: "Needs Review", value: pending }),
      h(StatTile, { label: "Failed Runs", value: failed }),
      h(StatTile, { label: "Artifacts", value: assets.length }),
    ),
    h(
      "section",
      { className: "grid two-column" },
      h(
        "div",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Capability Hubs"), h(Badge, { tone: "blue" }, "Product")),
        h(
          "div",
          { className: "panel-body grid workflow-grid" },
          h(CapabilityCard, { title: "Research", body: "Find papers, summarize claims, and manage citations.", action: "Open Research", onClick: () => setPage("research"), tone: "blue" }),
          h(CapabilityCard, { title: "Create", body: "Generate scripts, storyboards, captions, and videos.", action: "Open Create", onClick: () => setPage("create"), tone: "amber" }),
          h(CapabilityCard, { title: "Track", body: "Log meals, analyze nutrition, and review trends.", action: "Open Track", onClick: () => setPage("track"), tone: "green" }),
        ),
      ),
      h(
        "div",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Recent Activity")),
        h(
          "div",
          { className: "panel-body activity-list" },
          activity.map((item) =>
            h(
              "button",
              { key: item.id, className: "activity-item", onClick: item.action },
              h("strong", null, item.title),
              h("span", { className: "subtle" }, formatDate(item.at)),
            ),
          ),
          activity.length === 0 ? h(EmptyState, { title: "No activity yet", body: "Run a workflow to populate execution history." }) : null,
        ),
      ),
    ),
  );
}

function WorkflowsPage({ workflows, query, setQuery, openWorkflow }) {
  const [filter, setFilter] = useState("all");
  const filtered = workflows.filter((workflow) => {
    const text = `${workflow.name} ${workflow.description} ${workflow.kind} ${workflow.ai?.purpose || ""}`.toLowerCase();
    const matchesQuery = !query || text.includes(query.toLowerCase());
    const matchesFilter =
      filter === "all" ||
      (filter === "ai" && workflow.kind === "ai_workflow") ||
      (filter === "review" && workflow.ai?.human_review_required) ||
      (filter === "high" && workflow.ai?.risk_level === "high");
    return matchesQuery && matchesFilter;
  });

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Automations",
      title: "Workflow catalog",
      description: "Browse orchestration definitions and open the advanced fallback workspace for workflow-level operation.",
      actions: [h("button", { className: "button primary", key: "new" }, "New Workflow")],
    }),
    h(
      "section",
      { className: "panel" },
      h(
        "div",
        { className: "panel-header" },
        h("input", {
          className: "input",
          value: query,
          onChange: (event) => setQuery(event.target.value),
          placeholder: "Search workflow definitions",
        }),
      ),
      h(
        "div",
        { className: "panel-body" },
        h(FilterBar, { value: filter, onChange: setFilter, items: [["all", "All"], ["ai", "AI"], ["review", "Review Required"], ["high", "High Risk"]] }),
      ),
    ),
    h(
      "section",
      { className: "grid workflow-grid" },
      filtered.map((workflow) => h(WorkflowCard, { key: workflow.id, workflow, openWorkflow })),
      filtered.length === 0 ? h(EmptyState, { title: "No workflows match", body: "Adjust search or filters to show more workflow definitions." }) : null,
    ),
  );
}

function CapabilityCard({ title, body, action, onClick, tone }) {
  return h(
    "article",
    { className: "workflow-card" },
    h("div", { className: "card-title-row" }, h("h3", null, title), h(Badge, { tone }, "Hub")),
    h("p", { className: "subtle" }, body),
    h("button", { className: "button primary", onClick }, action),
  );
}

function DomainSummary({ items }) {
  return h(
    "section",
    { className: "grid metrics-grid" },
    items.map(([label, value]) => h(StatTile, { key: label, label, value })),
  );
}

function ResearchPage({ workflows, runs, openRun }) {
  const workflow = workflowByType(workflows, "paper") || workflows.find((item) => item.id === "paper-search") || sampleWorkflows[0];
  const researchRuns = runs.filter((run) => run.workflow_id === workflow.id);

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Research",
      title: "Research hub",
      description: "Search papers, build summaries, track citations, and keep research history without exposing workflow internals first.",
      actions: [
        h("button", { className: "button primary", key: "search" }, "New Paper Search"),
        h("button", { className: "button", key: "history", onClick: () => researchRuns[0] && openRun(researchRuns[0].id) }, "System Trace"),
      ],
    }),
    h(DomainSummary, {
      items: [
        ["Paper summaries", "3"],
        ["Citations", "18"],
        ["Needs review", "1"],
      ],
    }),
    h(WorkspacePage, { workflow, runs, openRun }),
  );
}

function CreatePage({ workflows, runs, openRun }) {
  const workflow = workflowByType(workflows, "video") || workflows.find((item) => item.id === "video-generator") || sampleWorkflows[1];
  const createRuns = runs.filter((run) => run.workflow_id === workflow.id);

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Create",
      title: "Content creation hub",
      description: "Turn source material into scripts, storyboards, captions, and reviewable video outputs.",
      actions: [
        h("button", { className: "button primary", key: "video" }, "New Video"),
        h("button", { className: "button", key: "trace", onClick: () => createRuns[0] && openRun(createRuns[0].id) }, "System Trace"),
      ],
    }),
    h(DomainSummary, {
      items: [
        ["Draft scripts", "2"],
        ["Storyboards", "1"],
        ["Pending approval", "1"],
      ],
    }),
    h(WorkspacePage, { workflow, runs, openRun }),
  );
}

function TrackPage({ workflows, runs, assets }) {
  const workflow = workflows.find((item) => item.id === "meal-logger") || workflows.find((item) => item.name.toLowerCase().includes("meal")) || sampleWorkflows[2];
  const template = workspaceTemplates["meal-logger"];

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Track",
      title: "Meal logging",
      description: "Log meals, estimate nutrition, inspect confidence flags, and export personal history.",
      actions: [h("button", { className: "button primary", key: "meal" }, "Log Meal")],
    }),
    h(DomainSummary, {
      items: [
        ["Meals today", "2"],
        ["Protein", "86g"],
        ["Low-confidence estimates", "1"],
      ],
    }),
    h(
      "section",
      { className: "workspace" },
      h("aside", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, "Meal Input")), h("div", { className: "panel-body workspace-stack" }, h(WorkspaceInputs, { workflow, template }))),
      h("section", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, "Nutrition Workspace")), h("div", { className: "panel-body" }, h(MealInteractionCenter))),
      h("aside", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, "Outputs"), h(StatusBadge, { status: "ready" })), h("div", { className: "panel-body workspace-stack" }, h(WorkspaceOutputs, { template }), h("p", { className: "subtle" }, `${assets.filter((asset) => asset.workflow_id === workflow.id).length} library items linked to meal tracking.`))),
    ),
  );
}

function AutomationsPage({ workflows, query, setQuery, selectedWorkflow, runs, openWorkflow, automationView, setAutomationView, openRun }) {
  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Automations",
      title: "Advanced automation surface",
      description: "Workflow definitions, schedules, templates, and the generic fallback workspace live here instead of the primary product navigation.",
      actions: [h("button", { className: "button primary", key: "new" }, "New Automation")],
    }),
    h(WorkspaceTabs, { value: automationView, onChange: setAutomationView, items: [["catalog", "Workflow Catalog"], ["workspace", "Fallback Workspace"], ["schedules", "Schedules"]] }),
    automationView === "catalog" ? h(WorkflowsPage, { workflows, query, setQuery, openWorkflow }) : null,
    automationView === "workspace" ? h(WorkspacePage, { workflow: selectedWorkflow, runs, openRun }) : null,
    automationView === "schedules" ? h(SchedulesPanel) : null,
  );
}

function SystemPage({ workflows, runs, selectedRun, setSelectedRunID, reviews, selectedReview, setSelectedReviewID, setReviews, reload, systemView, setSystemView }) {
  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "System",
      title: "Operator surfaces",
      description: "Runs, reviews, traces, model calls, plugins, and settings remain available for audit and debugging.",
    }),
    h(WorkspaceTabs, { value: systemView, onChange: setSystemView, items: [["runs", "Runs"], ["reviews", "Reviews"], ["traces", "Traces"], ["settings", "Settings"]] }),
    systemView === "runs" ? h(RunsPage, { runs, selectedRun, setSelectedRunID }) : null,
    systemView === "reviews" ? h(ReviewPage, { reviews, selectedReview, setSelectedReviewID, setReviews, reload }) : null,
    systemView === "traces" ? h(TraceExplorerPage, { runs }) : null,
    systemView === "settings" ? h(SettingsPage, { workflows }) : null,
  );
}

const workspaceRegistry = {
  default: GenericWorkspace,
  paper: PaperWorkspace,
  video: VideoWorkspace,
};

function WorkspacePage({ workflow, runs, openRun }) {
  if (!workflow) return h(EmptyState, { title: "No workflow selected", body: "Choose a workflow to open its workspace." });
  const currentRun = runs.find((run) => run.workflow_id === workflow.id) || sampleRuns.find((run) => run.workflow_id === workflow.id) || sampleRuns[0];
  const workspaceType = normalizeWorkspaceType(workflow.ai?.workspaceType);
  const WorkspaceComponent = workspaceRegistry[workspaceType] || workspaceRegistry.default;

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Workspace",
      title: workflow.name,
      description: workflow.ai?.purpose || workflow.description,
      actions: [
        h(Badge, { tone: "blue", key: "type" }, `Workspace: ${workspaceType}`),
        h(Badge, { tone: riskTone(workflow.ai?.risk_level), key: "risk" }, `Risk: ${workflow.ai?.risk_level || "unknown"}`),
        h(Badge, { tone: workflow.ai?.human_review_required ? "amber" : "gray", key: "review" }, workflow.ai?.human_review_required ? "Review required" : "Review optional"),
        h("button", { className: "button primary", key: "run" }, "Run"),
        h("button", { className: "button", key: "history", onClick: () => openRun(currentRun?.id) }, "Trace"),
      ],
    }),
    h(WorkspaceComponent, { workflow, run: currentRun }),
  );
}

function WorkspaceLayout({ left, centerTitle, centerTabs, center, rightStatus, right }) {
  return h(
    "section",
    { className: "workspace" },
    h("aside", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, "Inputs")), h("div", { className: "panel-body workspace-stack" }, left)),
    h("section", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, centerTitle), centerTabs), h("div", { className: "panel-body" }, center)),
    h("aside", { className: "panel" }, h("div", { className: "panel-header" }, h("h2", null, "Outputs"), rightStatus), h("div", { className: "panel-body workspace-stack" }, right)),
  );
}

function GenericWorkspace({ workflow, run }) {
  const template = workflowTemplate(workflow);
  const [tab, setTab] = useState("interaction");

  return h(WorkspaceLayout, {
    left: h(WorkspaceInputs, { workflow, template }),
    centerTitle: "Workflow Interaction",
    centerTabs: h(WorkspaceTabs, { value: tab, onChange: setTab, items: [["interaction", "Interaction"], ["observability", "Observability"]] }),
    center: tab === "interaction" ? h(GenericInteractionCenter, { workflow, run, template }) : h(EventTimeline, { run }),
    rightStatus: h(StatusBadge, { status: run?.status || "ready" }),
    right: h(WorkspaceOutputs, { template }),
  });
}

function PaperWorkspace({ workflow, run }) {
  const template = workflowTemplate(workflow);
  const [tab, setTab] = useState("research");

  return h(WorkspaceLayout, {
    left: h(WorkspaceInputs, { workflow, template }),
    centerTitle: "Paper Research",
    centerTabs: h(WorkspaceTabs, { value: tab, onChange: setTab, items: [["research", "Research"], ["execution", "Execution"], ["observability", "Observability"]] }),
    center: tab === "research" ? h(PaperInteractionCenter, { workflow }) : tab === "execution" ? h(ExecutionSteps, { steps: template.steps, run }) : h(EventTimeline, { run }),
    rightStatus: h(StatusBadge, { status: run?.status || "ready" }),
    right: h(WorkspaceOutputs, { template }),
  });
}

function VideoWorkspace({ workflow, run }) {
  const template = workflowTemplate(workflow);
  const [tab, setTab] = useState("production");

  return h(WorkspaceLayout, {
    left: h(WorkspaceInputs, { workflow, template }),
    centerTitle: "Video Production",
    centerTabs: h(WorkspaceTabs, { value: tab, onChange: setTab, items: [["production", "Production"], ["execution", "Execution"], ["observability", "Observability"]] }),
    center: tab === "production" ? h(VideoInteractionCenter, { workflow }) : tab === "execution" ? h(ExecutionSteps, { steps: template.steps, run }) : h(EventTimeline, { run }),
    rightStatus: h(StatusBadge, { status: run?.status || "ready" }),
    right: h(WorkspaceOutputs, { template }),
  });
}

function WorkspaceInputs({ workflow, template }) {
  return h(
    React.Fragment,
    null,
    template.inputFields.map(([label, value]) => h("div", { className: "field", key: label }, h("label", null, label), h("input", { className: "input", defaultValue: value }))),
    h("div", { className: "field" }, h("label", null, "Model"), h("select", { className: "select", defaultValue: workflow.ai?.models?.[0] || "" }, (workflow.ai?.models?.length ? workflow.ai.models : ["Not declared"]).map((model) => h("option", { key: model, value: model }, model)))),
    h("div", { className: "field" }, h("label", null, "Workspace Type"), h("input", { className: "input", value: workflow.ai?.workspaceType || "default", readOnly: true })),
    h("button", { className: "button primary" }, "Run Workflow"),
  );
}

function WorkspaceOutputs({ template }) {
  return h(
    React.Fragment,
    null,
    h(ArtifactList, { artifacts: template.artifacts }),
    h("div", { className: "field" }, h("label", null, "Preview"), h("div", { className: "artifact-preview" }, template.preview)),
    h("div", { className: "button-row" }, template.exports.map((item) => h("button", { className: "button", key: item }, item))),
  );
}

function WorkspaceTabs({ value, onChange, items }) {
  return h("div", { className: "tabs" }, items.map(([id, label]) => h("button", { key: id, className: `tab ${value === id ? "active" : ""}`, onClick: () => onChange(id) }, label)));
}

function GenericInteractionCenter({ workflow, run, template }) {
  return h(
    "div",
    { className: "workspace-stack" },
    h("div", { className: "empty-state" }, h("h3", null, "Generic workflow fallback"), h("p", { className: "subtle" }, "This workspace keeps interaction separate from the execution engine: inputs configure the run, the center guides the workflow, and observability stays available in the secondary tab.")),
    h(ExecutionSteps, { steps: template.steps, run }),
    h(ChatLayer, { workflow }),
  );
}

function PaperInteractionCenter() {
  return h(
    "div",
    { className: "workspace-stack" },
    h("div", { className: "interaction-grid" },
      h("div", { className: "interaction-card" }, h("h3", null, "Search Strategy"), h("p", { className: "subtle" }, "Query expansion, source selection, and recency filters drive paper discovery before the engine executes ranking nodes.")),
      h("div", { className: "interaction-card" }, h("h3", null, "Review Focus"), h("p", { className: "subtle" }, "Check unsupported claims, citation quality, source relevance, and summary accuracy before approving artifacts.")),
    ),
    h(
      "table",
      { className: "table" },
      h("thead", null, h("tr", null, h("th", null, "Candidate"), h("th", null, "Signal"), h("th", null, "Action"))),
      h("tbody", null,
        h("tr", null, h("td", null, "Tool-using agents for software tasks"), h("td", null, "High citation relevance"), h("td", null, h(Badge, { tone: "green" }, "Keep"))),
        h("tr", null, h("td", null, "Survey of autonomous coding assistants"), h("td", null, "Broad but useful context"), h("td", null, h(Badge, { tone: "amber" }, "Review"))),
        h("tr", null, h("td", null, "Unverified benchmark claim"), h("td", null, "Needs source check"), h("td", null, h(Badge, { tone: "red" }, "Flag"))),
      ),
    ),
  );
}

function VideoInteractionCenter() {
  return h(
    "div",
    { className: "workspace-stack" },
    h("div", { className: "interaction-grid" },
      h("div", { className: "interaction-card" }, h("h3", null, "Production Stages"), h("p", { className: "subtle" }, "Outline, script, storyboard, captions, preview render, and final export are presented as creative work products rather than raw node logs.")),
      h("div", { className: "interaction-card" }, h("h3", null, "Review Gates"), h("p", { className: "subtle" }, "Approve script before expensive rendering, then approve final media before export.")),
    ),
    h(
      "div",
      { className: "storyboard" },
      ["Hook", "Workflow Demo", "Review Gate", "Export"].map((scene, index) =>
        h("div", { className: "storyboard-frame", key: scene }, h("span", { className: "mono" }, `Scene ${index + 1}`), h("strong", null, scene), h("p", { className: "subtle" }, index === 0 ? "Open with the user goal." : index === 1 ? "Show inputs and execution." : index === 2 ? "Pause for approval." : "Generate final assets.")),
      ),
    ),
  );
}

function MealInteractionCenter() {
  return h(
    "div",
    { className: "workspace-stack" },
    h(
      "div",
      { className: "interaction-grid" },
      h("div", { className: "interaction-card" }, h("h3", null, "Parsed Foods"), h("p", { className: "subtle" }, "Chicken, rice, avocado, greens, and dressing were detected from the meal description.")),
      h("div", { className: "interaction-card" }, h("h3", null, "Confidence Flags"), h("p", { className: "subtle" }, "Portion estimate is medium confidence. Ask for clarification before using this for strict tracking.")),
    ),
    h(
      "table",
      { className: "table" },
      h("thead", null, h("tr", null, h("th", null, "Nutrient"), h("th", null, "Estimate"), h("th", null, "Confidence"))),
      h(
        "tbody",
        null,
        h("tr", null, h("td", null, "Protein"), h("td", null, "42g"), h("td", null, h(Badge, { tone: "green" }, "High"))),
        h("tr", null, h("td", null, "Carbs"), h("td", null, "58g"), h("td", null, h(Badge, { tone: "amber" }, "Medium"))),
        h("tr", null, h("td", null, "Fat"), h("td", null, "24g"), h("td", null, h(Badge, { tone: "amber" }, "Medium"))),
      ),
    ),
  );
}

function SchedulesPanel() {
  return h(
    "section",
    { className: "panel" },
    h("div", { className: "panel-header" }, h("h2", null, "Schedules")),
    h(
      "div",
      { className: "panel-body" },
      h(EmptyState, {
        title: "Schedule management",
        body: "Recurring workflow schedules stay in Automations because they are orchestration primitives, not primary user tasks.",
      }),
    ),
  );
}

function TraceExplorerPage({ runs }) {
  const aiEvents = runs.flatMap((run) =>
    (run.events || [])
      .filter((event) => event.type === "ai_model_call")
      .map((event) => ({
        ...event,
        run_id: run.id,
        workflow_name: run.workflow_name,
      })),
  );

  return h(
    "section",
    { className: "panel" },
    h("div", { className: "panel-header" }, h("h2", null, "Trace Explorer"), h(Badge, { tone: "blue" }, `${aiEvents.length} model calls`)),
    h(
      "div",
      { className: "panel-body" },
      aiEvents.length > 0
        ? h(
            "table",
            { className: "table" },
            h("thead", null, h("tr", null, h("th", null, "Workflow"), h("th", null, "Model"), h("th", null, "Prompt Hash"), h("th", null, "Latency"), h("th", null, "Status"))),
            h(
              "tbody",
              null,
              aiEvents.map((event, index) =>
                h(
                  "tr",
                  { key: `${event.run_id}-${index}` },
                  h("td", null, event.workflow_name),
                  h("td", null, event.fields?.model || "Unknown"),
                  h("td", { className: "mono" }, event.fields?.prompt_hash || "-"),
                  h("td", null, event.fields?.latency_ms ? `${event.fields.latency_ms}ms` : "-"),
                  h("td", null, h(StatusBadge, { status: event.fields?.status || "event" })),
                ),
              ),
            ),
          )
        : h(EmptyState, { title: "No model calls yet", body: "AI observability appears here after model-call events are recorded." }),
    ),
  );
}

function RunsPage({ runs, selectedRun, setSelectedRunID }) {
  const run = selectedRun || runs[0];
  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Runs",
      title: "Execution trace",
      description: "Inspect run metadata, AI model-call events, prompt hashes, token usage, latency, and artifacts.",
    }),
    h(
      "section",
      { className: "split" },
      h(
        "aside",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Run History")),
        h(
          "div",
          { className: "panel-body review-list" },
          runs.map((item) =>
            h(
              "button",
              { key: item.id, className: "review-item", onClick: () => setSelectedRunID(item.id) },
              h("span", { className: "review-main" }, h("strong", null, item.workflow_name), h("span", { className: "subtle mono" }, item.id)),
              h(StatusBadge, { status: item.status }),
            ),
          ),
        ),
      ),
      h(
        "section",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, run?.workflow_name || "Run"), run ? h(StatusBadge, { status: run.status }) : null),
        h("div", { className: "panel-body" }, run ? h(EventTimeline, { run }) : h(EmptyState, { title: "No run selected", body: "Select a run to inspect its event trace." })),
      ),
      h(
        "aside",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Run Metadata")),
        h("div", { className: "panel-body" }, run ? h(RunMetadata, { run }) : null),
      ),
    ),
  );
}

function ReviewPage({ reviews, selectedReview, setSelectedReviewID, setReviews, reload }) {
  const [decision, setDecision] = useState("approve");
  const [comment, setComment] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const review = selectedReview || reviews[0];

  async function submitDecision() {
    if (!review) return;
    setSubmitting(true);
    if (decision === "request edits") {
      setReviews((current) => current.map((item) => (item.id === review.id ? { ...item, status: "pending", comment: `Request edits: ${comment}` } : item)));
      setSubmitting(false);
      return;
    }
    const endpoint = decision === "reject" ? "reject" : "approve";
    try {
      await fetchData(`/reviews/${review.id}/${endpoint}`, {
        method: "POST",
        body: JSON.stringify({ reviewer: "operator", comment }),
      });
      await reload();
    } catch (error) {
      setReviews((current) => current.map((item) => (item.id === review.id ? { ...item, status: endpoint === "approve" ? "approved" : "rejected", comment } : item)));
    } finally {
      setSubmitting(false);
    }
  }

  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Human Review",
      title: "Review queue",
      description: "Approve, reject, or annotate AI outputs before downstream execution or export.",
    }),
    h(
      "section",
      { className: "split" },
      h(
        "aside",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Queue"), h(Badge, { tone: "amber" }, `${reviews.filter((item) => item.status === "pending").length} pending`)),
        h(
          "div",
          { className: "panel-body review-list" },
          reviews.map((item) =>
            h(
              "button",
              { key: item.id, className: "review-item", onClick: () => setSelectedReviewID(item.id) },
              h("span", { className: "review-main" }, h("strong", null, item.workflow_name), h("span", { className: "subtle" }, item.node_name || item.node_id)),
              h(StatusBadge, { status: item.status }),
            ),
          ),
        ),
      ),
      h(
        "section",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Review Item"), review ? h(StatusBadge, { status: review.status }) : null),
        h("div", { className: "panel-body" }, review ? h(ReviewItemDetail, { review }) : h(EmptyState, { title: "No review selected", body: "Select a review item to inspect it." })),
      ),
      h(
        "aside",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Decision")),
        h(
          "div",
          { className: "panel-body workspace-stack" },
          h(
            "div",
            { className: "decision-options" },
            ["approve", "reject", "request edits"].map((item) =>
              h("label", { key: item }, h("input", { type: "radio", name: "decision", checked: decision === item, onChange: () => setDecision(item) }), item),
            ),
          ),
          h("div", { className: "field" }, h("label", null, "Comment"), h("textarea", { className: "textarea", value: comment, onChange: (event) => setComment(event.target.value), placeholder: "Add review rationale" })),
          h("button", { className: `button ${decision === "reject" ? "danger" : "primary"}`, disabled: submitting || !review, onClick: submitDecision }, submitting ? "Submitting..." : "Submit Decision"),
        ),
      ),
    ),
  );
}

function AssetsPage({ assets }) {
  const [selectedID, setSelectedID] = useState(assets[0]?.id);
  const selected = assets.find((asset) => asset.id === selectedID) || assets[0];
  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Assets",
      title: "Artifacts and history",
      description: "Browse durable files and outputs created by workflow executions.",
    }),
    h(
      "section",
      { className: "grid two-column" },
      h(
        "div",
        { className: "grid asset-grid" },
        assets.map((asset) =>
          h(
            "button",
            { key: asset.id, className: "asset-card", onClick: () => setSelectedID(asset.id) },
            h("div", { className: "card-title-row" }, h("h3", null, asset.name), h(Badge, { tone: statusTone(asset.review_state) }, asset.review_state)),
            h("p", { className: "subtle" }, asset.workflow_name),
            h("div", { className: "badge-row" }, h(Badge, { tone: "gray" }, asset.type), h(Badge, { tone: "blue" }, formatDate(asset.created_at))),
          ),
        ),
      ),
      h(
        "aside",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Asset Preview")),
        h("div", { className: "panel-body" }, selected ? h(AssetDetail, { asset: selected }) : h(EmptyState, { title: "No assets", body: "Artifacts will appear after workflow runs." })),
      ),
    ),
  );
}

function SettingsPage({ workflows }) {
  const modelNames = Array.from(new Set(workflows.flatMap((workflow) => workflow.ai?.models || [])));
  return h(
    React.Fragment,
    null,
    h(PageHeader, {
      eyebrow: "Settings",
      title: "Platform configuration",
      description: "MVP settings for models, plugins, storage, runtime, and review policy.",
    }),
    h(
      "section",
      { className: "grid two-column" },
      h(
        "div",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Model Inventory")),
        h(
          "div",
          { className: "panel-body" },
          h(
            "table",
            { className: "table" },
            h("thead", null, h("tr", null, h("th", null, "Model"), h("th", null, "Used By"), h("th", null, "Status"))),
            h(
              "tbody",
              null,
              (modelNames.length ? modelNames : ["No models declared"]).map((model) =>
                h(
                  "tr",
                  { key: model },
                  h("td", null, model),
                  h("td", null, workflows.filter((workflow) => workflow.ai?.models?.includes(model)).length || "-"),
                  h("td", null, h(Badge, { tone: modelNames.length ? "green" : "gray" }, modelNames.length ? "Enabled" : "Missing")),
                ),
              ),
            ),
          ),
        ),
      ),
      h(
        "div",
        { className: "panel" },
        h("div", { className: "panel-header" }, h("h2", null, "Review Policies")),
        h(
          "div",
          { className: "panel-body settings-list" },
          h(SettingRow, { title: "Require review for high-risk workflows", enabled: true }),
          h(SettingRow, { title: "Log prompt hashes for AI model calls", enabled: true }),
          h(SettingRow, { title: "Require review before external export", enabled: false }),
          h(SettingRow, { title: "Expose token usage and latency in run traces", enabled: true }),
        ),
      ),
    ),
  );
}

function WorkflowCard({ workflow, openWorkflow }) {
  return h(
    "article",
    { className: "workflow-card" },
    h(
      "div",
      { className: "card-title-row" },
      h("div", null, h("h3", null, workflow.name), h("p", { className: "subtle" }, workflow.ai?.purpose || workflow.description)),
      h(StatusBadge, { status: workflow.status || "ready" }),
    ),
    h(
      "div",
      { className: "badge-row" },
      h(Badge, { tone: workflow.kind === "ai_workflow" ? "blue" : "gray" }, workflow.kind),
      h(Badge, { tone: "blue" }, `Workspace: ${workflow.ai?.workspaceType || "default"}`),
      h(Badge, { tone: riskTone(workflow.ai?.risk_level) }, `Risk: ${workflow.ai?.risk_level || "unknown"}`),
      h(Badge, { tone: workflow.ai?.human_review_required ? "amber" : "gray" }, workflow.ai?.human_review_required ? "Review" : "No gate"),
    ),
    h(
      "div",
      { className: "meta-list" },
      h("div", { className: "meta-row" }, h("span", null, "Models"), h("strong", null, workflowModels(workflow))),
      h("div", { className: "meta-row" }, h("span", null, "Nodes"), h("strong", null, workflow.node_count || 0)),
      h("div", { className: "meta-row" }, h("span", null, "Version"), h("strong", null, workflow.active_version || 1)),
    ),
    h("div", { className: "button-row" }, h("button", { className: "button primary", onClick: () => openWorkflow(workflow.id) }, "Open"), h("button", { className: "button" }, "Run")),
  );
}

function ExecutionSteps({ steps, run }) {
  const status = run?.status || "ready";
  return h(
    "div",
    { className: "step-list" },
    steps.map((step, index) => {
      const complete = status === "completed" || index < Math.max(1, Math.floor(steps.length / 2));
      const paused = status === "paused" && index === steps.length - 1;
      return h(
        "div",
        { className: "step", key: step },
        h("span", { className: "step-main" }, h("span", { className: "step-title" }, `${index + 1}. ${step}`), h("span", { className: "subtle" }, paused ? "Waiting for human decision" : complete ? "Completed or ready" : "Queued")),
        h(StatusBadge, { status: paused ? "pending" : complete ? "completed" : "ready" }),
      );
    }),
  );
}

function ChatLayer({ workflow }) {
  return h(
    "div",
    { className: "workspace-stack" },
    h("div", { className: "empty-state" }, h("h3", null, "Chat support layer"), h("p", { className: "subtle" }, "Use chat to adjust a run, explain output, or add operator notes. The workflow remains structured through inputs, steps, and artifacts.")),
    h("textarea", { className: "textarea", placeholder: `Ask ${workflow.name} to revise scope, explain a result, or draft a follow-up.` }),
    h("button", { className: "button" }, "Send Note"),
  );
}

function EventTimeline({ run }) {
  const [selectedIndex, setSelectedIndex] = useState(0);
  const events = run?.events || [];
  const selected = events[selectedIndex];
  return h(
    "div",
    { className: "grid two-column" },
    h(
      "div",
      { className: "event-list" },
      events.map((event, index) =>
        h(
          "button",
          { key: `${event.type}-${index}`, className: `event ${selectedIndex === index ? "active" : ""}`, onClick: () => setSelectedIndex(index) },
          h("span", { className: "event-main" }, h("span", { className: "event-title" }, event.type), h("span", { className: "subtle" }, `${formatDate(event.occurred_at)} ${event.node_id ? `- ${event.node_id}` : ""}`)),
          h(Badge, { tone: statusTone(event.fields?.status || event.type) }, event.fields?.status || "event"),
        ),
      ),
      events.length === 0 ? h(EmptyState, { title: "No events", body: "This run has no recorded events yet." }) : null,
    ),
    h(
      "div",
      null,
      h("h3", null, "Selected Event"),
      h("div", { style: { height: "10px" } }),
      selected ? h("pre", { className: "json-preview" }, JSON.stringify(selected, null, 2)) : h(EmptyState, { title: "No event selected", body: "Select an event to inspect metadata." }),
    ),
  );
}

function RunMetadata({ run }) {
  return h(
    "div",
    { className: "meta-list" },
    h("div", { className: "meta-row" }, h("span", null, "Run ID"), h("strong", { className: "mono" }, run.id)),
    h("div", { className: "meta-row" }, h("span", null, "Workflow"), h("strong", null, run.workflow_name)),
    h("div", { className: "meta-row" }, h("span", null, "Started"), h("strong", null, formatDate(run.started_at))),
    h("div", { className: "meta-row" }, h("span", null, "Duration"), h("strong", null, formatDuration(run.duration_ms))),
    h("div", { className: "meta-row" }, h("span", null, "Risk"), h("strong", null, run.ai?.risk_level || "unknown")),
    h("div", { className: "meta-row" }, h("span", null, "Models"), h("strong", null, run.ai?.models?.join(", ") || "From events")),
    h("div", { className: "field" }, h("label", null, "Input"), h("pre", { className: "json-preview" }, JSON.stringify(run.input || {}, null, 2))),
  );
}

function ReviewItemDetail({ review }) {
  return h(
    "div",
    { className: "review-output" },
    h("div", { className: "meta-list" }, h("div", { className: "meta-row" }, h("span", null, "Workflow"), h("strong", null, review.workflow_name)), h("div", { className: "meta-row" }, h("span", null, "Run"), h("strong", { className: "mono" }, review.run_id)), h("div", { className: "meta-row" }, h("span", null, "Node"), h("strong", null, review.node_name || review.node_id)), h("div", { className: "meta-row" }, h("span", null, "Created"), h("strong", null, formatDate(review.created_at)))),
    h("div", { className: "field" }, h("label", null, "Proposed Output"), h("div", { className: "artifact-preview" }, typeof review.proposed_output === "string" ? review.proposed_output : JSON.stringify(review.proposed_output, null, 2))),
    h("div", { className: "field" }, h("label", null, "AI Metadata"), h("pre", { className: "json-preview" }, JSON.stringify(review.context || {}, null, 2))),
  );
}

function AssetDetail({ asset }) {
  return h(
    "div",
    { className: "workspace-stack" },
    h("div", { className: "meta-list" }, h("div", { className: "meta-row" }, h("span", null, "Name"), h("strong", null, asset.name)), h("div", { className: "meta-row" }, h("span", null, "Type"), h("strong", null, asset.type)), h("div", { className: "meta-row" }, h("span", null, "Workflow"), h("strong", null, asset.workflow_name)), h("div", { className: "meta-row" }, h("span", null, "Run"), h("strong", { className: "mono" }, asset.run_id)), h("div", { className: "meta-row" }, h("span", null, "Review"), h("strong", null, asset.review_state))),
    h("div", { className: "artifact-preview" }, `Preview placeholder for ${asset.name}.\n\nWhen artifact serving is available, this panel can render markdown, JSON, video, captions, images, and export metadata.`),
    h("div", { className: "button-row" }, h("button", { className: "button" }, "Open Source Run"), h("button", { className: "button" }, "Export"), h("button", { className: "button danger" }, "Delete")),
  );
}

function ArtifactList({ artifacts }) {
  return h(
    "div",
    { className: "artifact-list" },
    artifacts.map((artifact) => h("div", { className: "artifact-row", key: artifact }, h("span", null, artifact), h(Badge, { tone: "gray" }, guessAssetType(artifact)))),
  );
}

function SettingRow({ title, enabled }) {
  return h("div", { className: "setting-row" }, h("span", null, title), h(Badge, { tone: enabled ? "green" : "gray" }, enabled ? "Enabled" : "Disabled"));
}

function FilterBar({ value, onChange, items }) {
  return h("div", { className: "filter-bar" }, items.map(([id, label]) => h("button", { key: id, className: `chip ${value === id ? "active" : ""}`, onClick: () => onChange(id) }, label)));
}

function StatTile({ label, value }) {
  return h("div", { className: "stat-tile" }, h("div", { className: "stat-label" }, label), h("div", { className: "stat-value" }, value));
}

function StatusBadge({ status }) {
  return h(Badge, { tone: statusTone(status) }, String(status || "unknown").replaceAll("_", " "));
}

function Badge({ tone = "gray", children }) {
  return h("span", { className: `badge ${tone}` }, children);
}

function EmptyState({ title, body }) {
  return h("div", { className: "empty-state" }, h("h3", null, title), h("p", { className: "subtle" }, body));
}

createRoot(document.getElementById("root")).render(h(App));

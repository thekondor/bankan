#!/usr/bin/env bash
set -euo pipefail

BANKAN="${BANKAN:-go run ./cmd/bankan}"
DEMO_DIR="${DEMO_DIR:-demo}"

rm -rf "$DEMO_DIR"
mkdir -p "$DEMO_DIR"

# ── Board 1: Website Redesign ─────────────────────────────────────────────────

b="$DEMO_DIR/website-redesign"
mkdir -p "$b"
$BANKAN board init "$b" --name "Website Redesign"

l_fe=$($BANKAN label add --board "$b" --name "Frontend"  --color "#3b82f6" | awk '{print $2}')
l_be=$($BANKAN label add --board "$b" --name "Backend"   --color "#10b981" | awk '{print $2}')
l_ux=$($BANKAN label add --board "$b" --name "UX"        --color "#f59e0b" | awk '{print $2}')
l_bg=$($BANKAN label add --board "$b" --name "Bug"       --color "#ef4444" | awk '{print $2}')

$BANKAN lane add "Backlog"     --board "$b"
$BANKAN lane add "Design"      --board "$b"
$BANKAN lane add "Development" --board "$b"
$BANKAN lane add "Done"        --board "$b"

$BANKAN card add --board "$b" --lane "Backlog" \
  --title "Update homepage hero section" \
  --body $'Refresh the hero with **new brand visuals** and updated copy from marketing.\n\n- Replace static background with a CSS gradient\n- Update headline to reflect Q3 campaign messaging' \
  --label "$l_fe" --label "$l_ux"

$BANKAN card add --board "$b" --lane "Backlog" \
  --title "Fix navigation mobile responsiveness" \
  --body $'Nav menu **breaks below 375 px** — hamburger icon overlaps the logo.\n\nRepro: open on iPhone SE; expected: _collapsible drawer_; actual: layout overflow.' \
  --label "$l_fe" --label "$l_bg"

$BANKAN card add --board "$b" --lane "Design" \
  --title "Design new color palette" \
  --body $'Work with brand team to produce a revised **5-color palette** for the redesign.\n\nDeliverable: Figma file with _primary_, _secondary_, _accent_, _neutral_, and _error_ swatches.' \
  --label "$l_ux"

$BANKAN card add --board "$b" --lane "Design" \
  --title "Create wireframes for checkout flow" \
  --body $'Cover the full funnel: **add-to-cart → shipping → payment → confirmation**.\n\nEach screen needs desktop _and_ mobile variants; use the new grid specs from the layout ticket.' \
  --label "$l_ux" --label "$l_fe"

$BANKAN card add --board "$b" --lane "Development" \
  --title "Implement responsive grid layout" \
  --body $'Replace the legacy **float-based layout** with CSS Grid across all product pages.\n\nTarget browsers: `Chrome 110+`, `Firefox 110+`, `Safari 16+`; add `@supports` fallback.' \
  --label "$l_fe"

$BANKAN card add --board "$b" --lane "Development" \
  --title "Set up CDN for static assets" \
  --body $'Route all images, fonts, and JS bundles through **CloudFront** to cut TTFB by ~40 %.\n\nRequired: CI invalidation script and a `Cache-Control` policy per asset type.' \
  --label "$l_be"

$BANKAN card add --board "$b" --lane "Development" \
  --title "Fix broken image links after migration" \
  --body $'**Several product images 404** after the S3 bucket was renamed last sprint.\n\nAffected paths match `^/media/products/.*\\.webp`; a one-off migration script should fix them all.' \
  --label "$l_bg" --label "$l_fe"

$BANKAN card add --board "$b" --lane "Done" \
  --title "Optimize database queries for product listing" \
  --body $'Reduced **p99 latency from 800 ms to 120 ms** by adding a composite index on `(category_id, updated_at DESC)`.\n\nAlso rewrote the N+1 loop to a single `JOIN` — query count per page load dropped from 47 to 3.' \
  --label "$l_be"

$BANKAN card add --board "$b" --lane "Done" \
  --title "Accessibility audit" \
  --body $'**WCAG 2.1 AA** audit completed via axe DevTools and a manual keyboard walkthrough.\n\n12 issues logged; _critical_: missing `aria-label` on icon buttons; _moderate_: insufficient colour contrast on disabled states.' \
  --label "$l_ux"

$BANKAN card add --board "$b" --lane "Done" \
  --title "Deploy staging environment" \
  --body $'Staging now **mirrors prod infra** and auto-deploys on every push to `main`.\n\nIncludes RDS snapshot restore, S3 sync, and a smoke-test suite gated by GitHub Actions.' \
  --label "$l_be"

# archived cards on Board 1
wr_arc1=$($BANKAN card add --board "$b" --lane "Backlog" \
  --title "Spike: evaluate Tailwind v4 migration" \
  --body $'Quick investigation into effort and risk of upgrading to **Tailwind v4**.\n\nDecided: too disruptive mid-redesign; defer to after launch.' \
  --label "$l_fe" | awk '{print $2}')
$BANKAN card archive "$wr_arc1" --board "$b"

wr_arc2=$($BANKAN card add --board "$b" --lane "Design" \
  --title "Explore dark-mode variant for landing page" \
  --body $'Stakeholders asked for a dark-mode hero option.\n\nDecision: _out of scope_ for current sprint; archived to revisit in Q4.' \
  --label "$l_ux" | awk '{print $2}')
$BANKAN card archive "$wr_arc2" --board "$b"

# ── Board 2: Mobile App v2.0 ──────────────────────────────────────────────────

b="$DEMO_DIR/mobile-app"
mkdir -p "$b"
$BANKAN board init "$b" --name "Mobile App v2.0"

l_ios=$($BANKAN label add --board "$b" --name "iOS"         --color "#007aff" | awk '{print $2}')
l_and=$($BANKAN label add --board "$b" --name "Android"     --color "#3ddc84" | awk '{print $2}')
l_crt=$($BANKAN label add --board "$b" --name "Critical"    --color "#ef4444" | awk '{print $2}')
l_enh=$($BANKAN label add --board "$b" --name "Enhancement" --color "#8b5cf6" | awk '{print $2}')

$BANKAN lane add "Ideas"       --board "$b"
$BANKAN lane add "In Progress" --board "$b"
$BANKAN lane add "Review"      --board "$b"
$BANKAN lane add "Shipped"     --board "$b"

$BANKAN card add --board "$b" --lane "Ideas" \
  --title "Add biometric login support" \
  --body $'Implement **Face ID / fingerprint auth** for both platforms using native APIs.\n\nUse `LocalAuthentication` on iOS and `BiometricPrompt` on Android; fall back to PIN if unavailable.' \
  --label "$l_ios" --label "$l_and" --label "$l_enh"

$BANKAN card add --board "$b" --lane "In Progress" \
  --title "Fix crash on deep link navigation" \
  --body $'App **segfaults** when a push-notification deep link opens a screen before init completes.\n\nStack trace points to `DeepLinkRouter.resolve()` — the fragment back-stack is empty on cold start.' \
  --label "$l_crt" --label "$l_and"

$BANKAN card add --board "$b" --lane "In Progress" \
  --title "Implement dark mode" \
  --body $'Follow **system colour scheme preference** and persist a user override in shared prefs.\n\nAll custom colours must have light _and_ dark variants in `colors.xml` / `Assets.xcassets`.' \
  --label "$l_enh" --label "$l_ios" --label "$l_and"

$BANKAN card add --board "$b" --lane "In Progress" \
  --title "Push notification delivery tracking" \
  --body $'Log **delivered** and **opened** events to the analytics pipeline for every push notification.\n\nUse `FirebaseMessaging` callbacks; forward events to Segment with `notification_id` as the key.' \
  --label "$l_enh"

$BANKAN card add --board "$b" --lane "Review" \
  --title "App Store review process checklist" \
  --body $'Verify **screenshots**, age ratings, export compliance declaration, and privacy nutrition label.\n\nReview takes 24–48 h; submit by _Friday_ to hit the Monday release window.' \
  --label "$l_ios"

$BANKAN card add --board "$b" --lane "Review" \
  --title "Profile cold start performance" \
  --body $'Target: **cold start under 1 s** on mid-range Android devices (Pixel 4a equivalent).\n\nUse Android Studio Profiler; instrument `Application.onCreate()` and the first `Activity.onResume()`.' \
  --label "$l_crt" --label "$l_and"

$BANKAN card add --board "$b" --lane "Review" \
  --title "Release 2.0.0 to beta testers" \
  --body $'Distribute via **TestFlight** (iOS) and the internal Play track (Android) to 500 opted-in testers.\n\nCollect crash reports for 48 h before promoting to production; monitor the _Crashlytics_ dashboard.' \
  --label "$l_ios" --label "$l_and"

$BANKAN card add --board "$b" --lane "Shipped" \
  --title "Ship v1.9 hotfix" \
  --body $'Patched **payment gateway timeout** that caused 3 % of checkout sessions to fail silently.\n\n100 % crash-free sessions restored within 2 h of rollout; no further regressions observed.' \
  --label "$l_crt"

# archived cards on Board 2
ma_arc1=$($BANKAN card add --board "$b" --lane "Ideas" \
  --title "Add Apple Watch companion app" \
  --body $'Prototype a minimal watch face showing daily summary stats.\n\nDecision: **deferred** — too far from core v2.0 scope; archived for backlog grooming.' \
  --label "$l_ios" --label "$l_enh" | awk '{print $2}')
$BANKAN card archive "$ma_arc1" --board "$b"

ma_arc2=$($BANKAN card add --board "$b" --lane "In Progress" \
  --title "Investigate ANR on settings screen" \
  --body $'**Application Not Responding** triggered on slow devices when opening settings on Android 10.\n\nRoot cause found: SharedPreferences read on main thread; refactor to async — _cancelled, superseded by a platform rewrite_.' \
  --label "$l_crt" --label "$l_and" | awk '{print $2}')
$BANKAN card archive "$ma_arc2" --board "$b"

ma_arc3=$($BANKAN card add --board "$b" --lane "Review" \
  --title "Old CI pipeline cleanup" \
  --body $'Remove legacy Bitrise workflows that were replaced by GitHub Actions in Q2.\n\nCompleted and archived — all references removed.' \
  --label "$l_enh" | awk '{print $2}')
$BANKAN card archive "$ma_arc3" --board "$b"

# ── Board 3: Q3 Infrastructure ────────────────────────────────────────────────

b="$DEMO_DIR/q3-infra"
mkdir -p "$b"
$BANKAN board init "$b" --name "Q3 Infrastructure"

l_sec=$($BANKAN label add --board "$b" --name "Security"    --color "#dc2626" | awk '{print $2}')
l_prf=$($BANKAN label add --board "$b" --name "Performance" --color "#f97316" | awk '{print $2}')
l_cst=$($BANKAN label add --board "$b" --name "Cost"        --color "#84cc16" | awk '{print $2}')
l_cmp=$($BANKAN label add --board "$b" --name "Compliance"  --color "#6366f1" | awk '{print $2}')

$BANKAN lane add "Todo"     --board "$b"
$BANKAN lane add "Doing"    --board "$b"
$BANKAN lane add "Blocked"  --board "$b"
$BANKAN lane add "Complete" --board "$b"

$BANKAN card add --board "$b" --lane "Todo" \
  --title "Upgrade Kubernetes to 1.28" \
  --body $'**Rolling upgrade** across all three clusters (dev, staging, prod) following the runbook.\n\nDrain nodes one AZ at a time; validate with `kubectl get nodes` after each batch.' \
  --label "$l_prf"

$BANKAN card add --board "$b" --lane "Todo" \
  --title "Rotate all service account keys" \
  --body $'**Quarterly rotation** of all GCP service account keys per security policy `SEC-07`.\n\nRun `scripts/rotate-sa-keys.sh`, update secrets in Vault, then redeploy affected workloads.' \
  --label "$l_sec" --label "$l_cmp"

$BANKAN card add --board "$b" --lane "Doing" \
  --title "Enable VPC flow logs" \
  --body $'Required for **incident forensics** and as SOC 2 CC6.6 evidence — must be live before the audit.\n\nLog destination: `s3://infra-logs/vpc-flow/`; retention 90 days; filter out `ACCEPT` on port 443.' \
  --label "$l_sec" --label "$l_cmp"

$BANKAN card add --board "$b" --lane "Doing" \
  --title "Reduce EC2 spend by rightsizing" \
  --body $'Analysing **30-day CPU and memory metrics** via CloudWatch; targeting a **20 % monthly saving**.\n\nCandidates: 14 `m5.2xlarge` instances running at < 15 % CPU — downsize to `m5.xlarge`.' \
  --label "$l_cst"

$BANKAN card add --board "$b" --lane "Blocked" \
  --title "Fix SSL cert renewal automation" \
  --body $'**Blocked**: DNS team has not granted the ACME validation role `Route53:ChangeResourceRecordSets`.\n\nWorkaround: manual renewal reminder set for _2026-06-01_; escalation ticket opened with DNS team.' \
  --label "$l_sec"

$BANKAN card add --board "$b" --lane "Complete" \
  --title "Migrate logs to cheaper storage tier" \
  --body $'Moved **3 TB of logs** older than 30 days to S3 Glacier Instant Retrieval; saves ~400 USD/month.\n\nLifecycle rule applied to `s3://infra-logs/*`; verified with `aws s3api get-bucket-lifecycle-configuration`.' \
  --label "$l_cst" --label "$l_prf"

$BANKAN card add --board "$b" --lane "Complete" \
  --title "Complete SOC 2 control mapping" \
  --body $'All **64 controls** mapped to evidence artifacts and owner teams; audit evidence package collected.\n\n_Next step_: external auditor review scheduled for 2026-06-15; controls dashboard available in Vanta.' \
  --label "$l_cmp" --label "$l_sec"

# archived cards on Board 3
qi_arc1=$($BANKAN card add --board "$b" --lane "Todo" \
  --title "Evaluate HashiCorp Vault OSS → Enterprise migration" \
  --body $'Spike to assess licensing cost vs. feature gain of Vault Enterprise.\n\nDecision: **not worth it** at current scale; archived.' \
  --label "$l_sec" --label "$l_cst" | awk '{print $2}')
$BANKAN card archive "$qi_arc1" --board "$b"

qi_arc2=$($BANKAN card add --board "$b" --lane "Doing" \
  --title "Set up Prometheus alerting for disk usage" \
  --body $'Add `node_disk_used_percent > 85` alert firing to PagerDuty.\n\nSuperseded by the new observability platform rollout — archived to avoid duplication.' \
  --label "$l_prf" | awk '{print $2}')
$BANKAN card archive "$qi_arc2" --board "$b"

# ── Board 4: Internal Planning (hidden) ───────────────────────────────────────

b="$DEMO_DIR/internal-planning"
mkdir -p "$b"
$BANKAN board init "$b" --name "Internal Planning"

l_eng=$($BANKAN label add --board "$b" --name "Engineering" --color "#6366f1" | awk '{print $2}')
l_prd=$($BANKAN label add --board "$b" --name "Product"     --color "#f59e0b" | awk '{print $2}')
l_ops=$($BANKAN label add --board "$b" --name "Ops"         --color "#10b981" | awk '{print $2}')
l_hig=$($BANKAN label add --board "$b" --name "High Prio"   --color "#ef4444" | awk '{print $2}')

$BANKAN lane add "Now"      --board "$b"
$BANKAN lane add "Next"     --board "$b"
$BANKAN lane add "Later"    --board "$b"
$BANKAN lane add "Parked"   --board "$b"
$BANKAN lane add "Done"     --board "$b"

$BANKAN card add --board "$b" --lane "Now" \
  --title "Finalize Q3 headcount plan" \
  --body $'Engineering hiring plan due to HR by end of week.\n\n- 2 senior backend roles\n- 1 SRE\n- 1 product designer (shared with Design org)' \
  --label "$l_eng" --label "$l_hig"

$BANKAN card add --board "$b" --lane "Now" \
  --title "Review and sign contractor SOWs" \
  --body $'Three active contractor SOWs need signature before billing cycle closes.\n\nForward signed copies to legal@company.com and update the vendor tracker.' \
  --label "$l_ops" --label "$l_hig"

$BANKAN card add --board "$b" --lane "Now" \
  --title "Define OKRs for Q3 engineering" \
  --body $'Align with CTO on two or three key engineering OKRs.\n\nDraft due in all-hands slide deck by _2026-06-03_.' \
  --label "$l_eng" --label "$l_prd"

$BANKAN card add --board "$b" --lane "Next" \
  --title "Plan team off-site logistics" \
  --body $'Book venue, travel, and accommodation for the August engineering off-site (12 people).\n\nBudget: €2 500; location shortlist in Notion.' \
  --label "$l_ops"

$BANKAN card add --board "$b" --lane "Next" \
  --title "Set up internal demo environment" \
  --body $'Spin up a long-lived demo cluster pointing at sanitised production data.\n\nRequired for sales demos and onboarding new PMs.' \
  --label "$l_eng" --label "$l_ops"

$BANKAN card add --board "$b" --lane "Next" \
  --title "Draft product roadmap H2 2026" \
  --body $'Consolidate input from sales, support, and engineering into a ranked roadmap.\n\nTarget: share draft with stakeholders by _2026-06-15_.' \
  --label "$l_prd" --label "$l_hig"

$BANKAN card add --board "$b" --lane "Later" \
  --title "Evaluate observability platform options" \
  --body $'Compare **Grafana Cloud**, **Honeycomb**, and **Datadog** on cost, cardinality limits, and query UX.\n\nNo urgent timeline; revisit in Q4 budget planning.' \
  --label "$l_eng" --label "$l_ops"

$BANKAN card add --board "$b" --lane "Later" \
  --title "Internal knowledge-base migration" \
  --body $'Move legacy Confluence space to Notion.\n\nEstimated 3-week effort; low urgency while current space is still functional.' \
  --label "$l_ops"

$BANKAN card add --board "$b" --lane "Parked" \
  --title "Investigate on-prem deployment option" \
  --body $'One enterprise prospect asked about self-hosted option.\n\nParked until there are at least two qualified leads with this requirement.' \
  --label "$l_eng" --label "$l_prd"

$BANKAN card add --board "$b" --lane "Done" \
  --title "Set up quarterly business review template" \
  --body $'Created reusable slide deck template for QBRs; shared in the company Google Drive.' \
  --label "$l_ops"

$BANKAN card add --board "$b" --lane "Done" \
  --title "Onboard two new backend engineers" \
  --body $'Onboarding docs updated, 90-day plans agreed, dev environments set up.\n\nBoth engineers are ramped — closed.' \
  --label "$l_eng"

$BANKAN board hide "internal-planning" --root "$DEMO_DIR"

# ── View Board 1: Frontend Sprint (active) — filtered from Website Redesign ───

vb1="$DEMO_DIR/frontend-sprint"
mkdir -p "$vb1"
$BANKAN board view create "$vb1" \
  --parent "$DEMO_DIR/website-redesign" \
  --label "$l_fe" \
  --name "Frontend Sprint"
$BANKAN board view sync --board "$vb1"

# add a view-only lane and move a stub into it
$BANKAN lane add "Sprint Ice Box" --board "$vb1"

# ── View Board 2: Hotfix Sprint (archived with archive-label) — filtered from Mobile App ──

vb2="$DEMO_DIR/hotfix-sprint"
mkdir -p "$vb2"
$BANKAN board view create "$vb2" \
  --parent "$DEMO_DIR/mobile-app" \
  --label "$l_crt" \
  --name "Hotfix Sprint v1.9"
$BANKAN board view sync --board "$vb2"
$BANKAN board view archive --board "$vb2" --archive-label

echo ""
echo "Demo boards created in $DEMO_DIR/"
echo "  Boards:      website-redesign, mobile-app, q3-infra"
echo "  Hidden:      internal-planning"
echo "  View active: frontend-sprint (filtered: Frontend label on website-redesign)"
echo "  View archived: hotfix-sprint (filtered: Critical label on mobile-app, label prefixed)"
echo "  Run: mise run demo-serve"

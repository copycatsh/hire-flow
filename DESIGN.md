# Design System — hire-flow

## Product Context
- **What this is:** AI-powered hiring platform where clients post jobs, get matched with freelancers, propose contracts, and manage payments
- **Who it's for:** Businesses hiring freelance talent (client app) and freelancers finding work (freelancer app)
- **Space/industry:** Talent marketplace / hiring SaaS (peers: Toptal, Upwork, Deel, Remote)
- **Project type:** Web app / dashboard (cards, data tables, forms, status flows)

## Aesthetic Direction
- **Direction:** Refined Modern — precision of Linear, warmth of Stripe, restraint of Vercel
- **Decoration level:** Intentional — subtle card shadows, warm surface layering, depth through elevation. No gradients on surfaces, no background patterns.
- **Mood:** Premium but approachable. A platform you trust with your hiring, not an admin panel you're stuck with. Confident whitespace, warm neutrals, intentional motion.
- **Reference sites:** Stripe Dashboard (warmth + clarity), Linear (precision + polish), Vercel (restraint + confidence)

## Typography
- **Display/Hero:** Instrument Sans — clean geometric with subtle character, distinctive but professional
- **Body:** DM Sans — highly readable at all sizes, optical sizing support, pairs naturally with Instrument Sans
- **UI/Labels:** DM Sans (weight 500)
- **Data/Tables:** Geist Mono — tabular-nums, modern monospace for clean number alignment
- **Code:** Geist Mono
- **Loading:** Google Fonts CDN
  ```html
  <link href="https://fonts.googleapis.com/css2?family=DM+Sans:ital,opsz,wght@0,9..40,300;0,9..40,400;0,9..40,500;0,9..40,600;0,9..40,700;1,9..40,400&family=Instrument+Sans:wght@400;500;600;700&family=Geist+Mono:wght@400;500;600&display=swap" rel="stylesheet">
  ```
- **Scale:**
  - Display XL: 48px / bold / -0.03em tracking / 1.1 line-height
  - Display LG: 32px / semibold / -0.02em / 1.2
  - Display MD: 24px / semibold / -0.01em / 1.3
  - Display SM: 18px / semibold / -0.01em / 1.3
  - Body: 16px / regular / 1.6
  - Body SM: 13px / regular / 1.5
  - Caption: 12px / medium / 1.4
  - Mono: 14px / regular / 1.5 (tabular-nums)
  - Labels: 12px / medium / uppercase / 0.04-0.08em tracking

## Color

### Approach: Balanced
Primary blue for trust, warm amber accent for personality. Slate-based neutrals add warmth to every surface.

### Light Mode
| Token | Hex | Usage |
|-------|-----|-------|
| Primary | #1D4ED8 | Main actions, links, focus rings |
| Primary Hover | #1E40AF | Button hover state |
| Primary Light | #EFF6FF | Primary backgrounds, selected states |
| Primary 500 | #3B82F6 | Active nav border, interactive highlights |
| Accent | #D97706 | Premium indicators, match scores, highlights |
| Accent Hover | #B45309 | Accent button hover |
| Accent Light | #FFFBEB | Accent badge backgrounds |
| Success | #059669 | Active status, confirmations |
| Success BG | #ECFDF5 | Success alert/badge background |
| Warning | #D97706 | Pending status, cautions |
| Warning BG | #FFFBEB | Warning alert/badge background |
| Error | #DC2626 | Errors, declined status, destructive actions |
| Error BG | #FEF2F2 | Error alert/badge background |
| Info | #2563EB | Informational messages |
| Info BG | #EFF6FF | Info alert background |
| Background | #FFFFFF | Page background, top nav, cards |
| BG Subtle | #F8FAFC | Secondary surfaces (main content area behind cards) |
| BG Muted | #F1F5F9 | Tertiary surfaces, table headers, tag backgrounds |
| Border | #E2E8F0 | Default borders, card borders, dividers |
| Border Strong | #CBD5E1 | Input borders, stronger dividers |
| Text | #0F172A | Primary text (warm slate-900) |
| Text Secondary | #475569 | Secondary text, labels (slate-600) |
| Text Tertiary | #94A3B8 | Placeholders, meta text (slate-400) |
| Text Inverse | #FFFFFF | Text on dark/primary backgrounds |

### Dark Mode
| Token | Hex | Usage |
|-------|-----|-------|
| Primary | #60A5FA | Main actions (blue-400, lightened for dark bg) |
| Primary Hover | #93C5FD | Hover state (blue-300) |
| Primary Light | #172554 | Primary backgrounds (blue-950) |
| Accent | #FBBF24 | Premium indicators (amber-400) |
| Accent Light | #451A03 | Accent backgrounds |
| Success | #34D399 | Active status (emerald-400) |
| Success BG | #064E3B | Success backgrounds |
| Warning | #FBBF24 | Pending status (amber-400) |
| Warning BG | #451A03 | Warning backgrounds |
| Error | #F87171 | Errors (red-400) |
| Error BG | #450A0A | Error backgrounds |
| Info | #60A5FA | Informational (blue-400) |
| Info BG | #172554 | Info backgrounds |
| Background | #0F172A | Page background (slate-900) |
| BG Subtle | #1E293B | Secondary surfaces (slate-800) |
| BG Muted | #334155 | Tertiary surfaces (slate-700) |
| Border | #334155 | Default borders (slate-700) |
| Border Strong | #475569 | Stronger dividers (slate-600) |
| Text | #F1F5F9 | Primary text (slate-100) |
| Text Secondary | #94A3B8 | Secondary text (slate-400) |
| Text Tertiary | #64748B | Placeholders, meta (slate-500) |

## Spacing
- **Base unit:** 4px
- **Density:** Spacious — generous whitespace between sections, comfortable internal padding
- **Scale:**
  - 2xs: 2px
  - xs: 4px
  - sm: 8px
  - md: 16px
  - lg: 24px
  - xl: 32px
  - 2xl: 48px
  - 3xl: 64px
- **Card internal padding:** 24px
- **Section gap:** 32-48px between major sections
- **Page padding:** 48px vertical, 32px horizontal

## Layout
- **Approach:** Grid-disciplined — consistent structure, card-based data presentation, top navigation
- **Navigation:** Top horizontal nav bar (64px height, white/bg background, subtle bottom border). NO sidebar.
- **Grid:** 12 columns, 20px gutter
- **Max content width:** 1280px (centered)
- **Card grid:** auto-fill, minmax(340px, 1fr)
- **Border radius:**
  - sm: 6px (buttons, inputs, tags)
  - md: 8px (cards, stat boxes)
  - lg: 12px (modals, larger containers, table wrappers)
  - full: 9999px (badges, pills, avatars, toggle)

## Motion
- **Approach:** Intentional — subtle animations that add polish and signal design quality
- **Easing:** enter(ease-out) exit(ease-in) move(ease-in-out)
- **Duration:**
  - micro: 50-100ms (hover states, focus rings, color transitions)
  - short: 150ms (button press, toggle, badge appear)
  - medium: 200-300ms (card hover lift, panel open/close)
  - long: 400-500ms (page entrance animations, staggered card reveals)
- **Patterns:**
  - Card entrance: staggered fade-up (translateY(16px) → 0, opacity 0 → 1) with 50ms stagger
  - Card hover: translateY(-3px) + shadow elevation increase
  - Page transitions: simple opacity crossfade (200ms)
  - Loading states: skeleton shimmer (not spinners, not opacity pulse)
  - Focus rings: 3px primary-light ring with short transition

## Shadows
- **sm:** 0 1px 2px rgba(0,0,0,0.05) — default card state
- **md:** 0 4px 6px -1px rgba(0,0,0,0.07), 0 2px 4px -2px rgba(0,0,0,0.05) — elevated elements, hover
- **lg:** 0 10px 15px -3px rgba(0,0,0,0.08), 0 4px 6px -4px rgba(0,0,0,0.04) — modals, dropdowns
- **card-hover:** 0 10px 25px -5px rgba(0,0,0,0.1), 0 4px 10px -4px rgba(0,0,0,0.06) — card hover state
- **Usage:** Cards use sm shadow by default, md on hover. Borders + shadows together for card definition.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-21 | Initial design system created | Created by /design-consultation based on competitive research |
| 2026-03-21 | Instrument Sans for display | Distinctive but professional, avoids overused Inter/Poppins |
| 2026-03-21 | IBM-style blue (#0F62FE) | Trust signal, professional, avoids SaaS-purple cliche |
| 2026-03-21 | **REDESIGN: Industrial → Refined Modern** | M8 client app looked like admin panel. Shifted to modern client-facing SaaS aesthetic |
| 2026-03-21 | Top nav replaces dark sidebar | Recovers 240px horizontal space, modern SaaS standard (Stripe, Linear, Vercel) |
| 2026-03-21 | Primary shifted #0F62FE → #1D4ED8 | Warmer, deeper royal blue. More confident than IBM blue, still professional |
| 2026-03-21 | Added warm amber accent (#D97706) | Distinctive in hiring space. Signals premium/opportunity. Used for match scores, badges |
| 2026-03-21 | Pure grays → warm slate neutrals | #F8FAFC/#F1F5F9/#E2E8F0 add subtle warmth to every surface without being noticeable |
| 2026-03-21 | Card-based layouts replace plain tables | Richer visual hierarchy, hover interactions, better for scanning job/freelancer data |
| 2026-03-21 | Border-radius softened (4/6/8 → 6/8/12) | Slightly softer without becoming bubbly. Matches Stripe/Linear aesthetic |
| 2026-03-21 | Motion: minimal → intentional | Staggered card entrances, hover lifts, skeleton loading. Signals design quality |
| 2026-03-21 | Density: comfortable → spacious | More whitespace says "we respect your time." Premium feel over data density |
| 2026-03-21 | Body font size 15px → 16px | Better readability, more breathing room, standard modern SaaS size |

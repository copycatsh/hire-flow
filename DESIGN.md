# Design System — hire-flow

## Product Context
- **What this is:** AI-powered hiring platform where clients post jobs, get matched with freelancers, propose contracts, and manage payments
- **Who it's for:** Businesses and clients hiring freelance talent
- **Space/industry:** Talent marketplace / hiring SaaS (peers: Toptal, Upwork, Deel, Remote)
- **Project type:** Web app / dashboard (data tables, forms, cards, status flows)

## Aesthetic Direction
- **Direction:** Industrial/Utilitarian — function-first, data-dense, clean
- **Decoration level:** Intentional — subtle borders, minimal shadows, clean card edges. No gradients, no patterns.
- **Mood:** Sharp, professional, efficient. Feels like a modern power tool, not a generic marketplace. Trustworthy without being corporate-boring.
- **Reference sites:** Toptal (professional tone), Linear (industrial SaaS benchmark), Remote.com (talent space context)

## Typography
- **Display/Hero:** Instrument Sans — clean geometric with subtle character, distinctive but professional. Not overused.
- **Body:** DM Sans — highly readable at all sizes, pairs beautifully with Instrument Sans. Optical sizing support.
- **UI/Labels:** DM Sans (same as body, weight 500)
- **Data/Tables:** Geist Mono — tabular-nums, modern monospace. Clean alignment for numbers and dates.
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
  - Body: 15px / regular / 1.6
  - Body SM: 13px / regular / 1.5
  - Caption: 12px / medium / 1.4
  - Mono: 13px / regular / 1.5 (tabular-nums)
  - Labels: 12px / medium / uppercase / 0.04-0.08em tracking

## Color

### Approach: Restrained
One accent + neutrals. Color is rare and meaningful.

### Light Mode
| Token | Hex | Usage |
|-------|-----|-------|
| Primary | #0F62FE | Main actions, links, focus rings |
| Primary Hover | #0043CE | Button hover state |
| Primary Light | #EDF5FF | Primary backgrounds, focus ring outer |
| Success | #198038 | Active status, confirmations |
| Success BG | #DEFBE6 | Success alert/badge background |
| Warning | #B28600 | Pending status, cautions |
| Warning BG | #FFF8E1 | Warning alert/badge background |
| Error | #DA1E28 | Errors, declined status, destructive actions |
| Error BG | #FFF1F1 | Error alert/badge background |
| Info | #0043CE | Informational messages |
| Info BG | #EDF5FF | Info alert background |
| Background | #FFFFFF | Page background |
| BG Subtle | #FAFAFA | Secondary surfaces (main content area) |
| BG Muted | #F4F4F4 | Tertiary surfaces, table headers |
| Border | #E0E0E0 | Default borders |
| Border Strong | #C6C6C6 | Input borders, stronger dividers |
| Text | #161616 | Primary text |
| Text Secondary | #525252 | Secondary text, labels |
| Text Placeholder | #A8A8A8 | Input placeholders |
| Text Inverse | #FFFFFF | Text on dark backgrounds |

### Dark Mode
| Token | Hex | Usage |
|-------|-----|-------|
| Primary | #78A9FF | Main actions (lightened for dark bg) |
| Primary Hover | #A6C8FF | Hover state |
| Primary Light | #001D6C | Primary backgrounds |
| Success | #42BE65 | Active status |
| Warning | #F1C21B | Pending status |
| Error | #FF8389 | Errors |
| Info | #78A9FF | Informational |
| Background | #0A0A0A | Page background |
| BG Subtle | #111111 | Secondary surfaces |
| BG Muted | #1A1A1A | Tertiary surfaces |
| Border | #2A2A2A | Default borders |
| Border Strong | #404040 | Stronger dividers |
| Text | #F4F4F4 | Primary text |
| Text Secondary | #A8A8A8 | Secondary text |

### Sidebar (both modes)
| Token | Hex | Usage |
|-------|-----|-------|
| Sidebar BG | #161616 (light) / #000000 (dark) | Navigation background |
| Sidebar Text | #C6C6C6 / #A8A8A8 | Nav item text |
| Sidebar Active | #FFFFFF / #F4F4F4 | Active nav item |
| Sidebar Hover | #262626 / #1A1A1A | Hover background |

## Spacing
- **Base unit:** 4px
- **Density:** Comfortable
- **Scale:**
  - 2xs: 2px
  - xs: 4px
  - sm: 8px
  - md: 16px
  - lg: 24px
  - xl: 32px
  - 2xl: 48px
  - 3xl: 64px

## Layout
- **Approach:** Grid-disciplined — consistent 12-column grid, sidebar navigation, predictable alignment
- **Grid:** 12 columns, 16px gutter
- **Sidebar:** 240px fixed width, dark background
- **Max content width:** 1120px (within main content area)
- **Border radius:**
  - sm: 4px (cards, containers, inputs)
  - md: 6px (buttons)
  - lg: 8px (modals, larger containers)
  - full: 9999px (badges, pills, toggle)

## Motion
- **Approach:** Minimal-functional — only transitions that aid comprehension
- **Easing:** enter(ease-out) exit(ease-in) move(ease-in-out)
- **Duration:**
  - micro: 50-100ms (hover states, focus rings)
  - short: 150ms (button press, toggle, color change)
  - medium: 200ms (panel open/close, page transition)
- **Rules:**
  - No decorative animations
  - No entrance animations on page load
  - Loading states use subtle opacity pulse, not spinners
  - Page transitions use simple opacity fade

## Shadows
- **sm:** 0 1px 2px rgba(0,0,0,0.05) — cards, dropdowns
- **md:** 0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.04) — elevated elements
- **Usage:** Prefer borders over shadows. Shadows only for elevation (dropdowns, modals).

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-21 | Initial design system created | Created by /design-consultation based on competitive research (Toptal, Remote.com, Linear) and product context |
| 2026-03-21 | Instrument Sans for display | Distinctive but professional, avoids overused Inter/Poppins |
| 2026-03-21 | Sharp border-radius (4-6px) | Differentiate from bubbly marketplace look, feel more like a power tool |
| 2026-03-21 | Dark sidebar navigation | Creates hierarchy, feels premium, contrasts with light content area |
| 2026-03-21 | IBM-style blue (#0F62FE) | Trust signal, professional, avoids SaaS-purple cliche |
| 2026-03-21 | No Zustand for state | React Context sufficient for auth state, YAGNI |

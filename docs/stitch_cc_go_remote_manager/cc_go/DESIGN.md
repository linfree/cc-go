---
name: cc-go
colors:
  surface: '#031427'
  surface-dim: '#031427'
  surface-bright: '#2a3a4f'
  surface-container-lowest: '#000f21'
  surface-container-low: '#0b1c30'
  surface-container: '#102034'
  surface-container-high: '#1b2b3f'
  surface-container-highest: '#26364a'
  on-surface: '#d3e4fe'
  on-surface-variant: '#c6c6cd'
  inverse-surface: '#d3e4fe'
  inverse-on-surface: '#213145'
  outline: '#909097'
  outline-variant: '#45464d'
  surface-tint: '#bec6e0'
  primary: '#bec6e0'
  on-primary: '#283044'
  primary-container: '#0f172a'
  on-primary-container: '#798098'
  inverse-primary: '#565e74'
  secondary: '#4edea3'
  on-secondary: '#003824'
  secondary-container: '#00a572'
  on-secondary-container: '#00311f'
  tertiary: '#ffb95f'
  on-tertiary: '#472a00'
  tertiary-container: '#251400'
  on-tertiary-container: '#b47300'
  error: '#ffb4ab'
  on-error: '#690005'
  error-container: '#93000a'
  on-error-container: '#ffdad6'
  primary-fixed: '#dae2fd'
  primary-fixed-dim: '#bec6e0'
  on-primary-fixed: '#131b2e'
  on-primary-fixed-variant: '#3f465c'
  secondary-fixed: '#6ffbbe'
  secondary-fixed-dim: '#4edea3'
  on-secondary-fixed: '#002113'
  on-secondary-fixed-variant: '#005236'
  tertiary-fixed: '#ffddb8'
  tertiary-fixed-dim: '#ffb95f'
  on-tertiary-fixed: '#2a1700'
  on-tertiary-fixed-variant: '#653e00'
  background: '#031427'
  on-background: '#d3e4fe'
  surface-variant: '#26364a'
typography:
  headline-lg:
    fontFamily: Inter
    fontSize: 30px
    fontWeight: '700'
    lineHeight: 36px
    letterSpacing: -0.02em
  headline-md:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 16px
  code-block:
    fontFamily: JetBrains Mono
    fontSize: 13px
    fontWeight: '400'
    lineHeight: 20px
  label-mono:
    fontFamily: JetBrains Mono
    fontSize: 11px
    fontWeight: '500'
    lineHeight: 14px
    letterSpacing: 0.05em
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
spacing:
  base: 4px
  xs: 0.25rem
  sm: 0.5rem
  md: 1rem
  lg: 1.5rem
  xl: 2rem
  sidebar-width: 240px
  max-width: 1440px
---

## Brand & Style

The brand identity of the design system is rooted in the "Modern Terminal" aesthetic—a hybrid of high-utility command-line interfaces and sophisticated modern web applications. It targets a developer audience that values efficiency, precision, and clarity. 

The personality is technical and authoritative yet accessible. By blending a minimalist layout with data-dense components, the UI evokes a sense of deep control over remote processes. The emotional response is one of reliability and "flow state" productivity. The design style leans into **Minimalism** with a touch of **Glassmorphism** for depth, utilizing crisp borders and high-contrast text to ensure information is scannable at a glance.

## Colors

The palette is optimized for long-duration technical work, utilizing a deep charcoal base to reduce eye strain.

- **Primary & Background**: The foundation is Deep Slate (#0f172a), providing a high-contrast canvas for white text and vibrant accents.
- **Accents (Status)**:
    - **Emerald (#10b981)**: Active sessions, successful deployments, and healthy status.
    - **Amber (#f59e0b)**: Pending permissions, warnings, or rate-limiting alerts.
    - **Rose (#f43f5e)**: Connection errors, failed commands, and critical logs.
    - **Slate (#64748b)**: Idle states, disconnected nodes, and secondary metadata.
- **Surfaces**: Secondary containers use a slightly lighter Slate (#1e293b) to create subtle separation without breaking the dark-mode immersion.

## Typography

This design system employs a dual-font strategy to distinguish between UI orchestration and technical data.

- **Interface Text**: **Inter** is used for all navigational elements, headers, and descriptive text. Its neutral, systematic nature ensures that the "app" wrapper remains unobtrusive.
- **Technical Data**: **JetBrains Mono** is utilized for session IDs, terminal logs, code snippets, and status labels. This monospaced font provides the necessary alignment for reading tabular data and logs.

Typography is scaled to be compact (data-dense) while maintaining legibility through generous line heights in log views.

## Layout & Spacing

The layout follows a **Fixed Sidebar + Fluid Content** model. The UI is designed to handle high volumes of concurrent information through a grid-based card system.

- **Sidebar**: A 240px fixed navigation rail on the left for session switching and environment management.
- **Main Content**: Uses a 12-column grid. On desktop, cards for "Active Processes" might span 8 columns, while "Metrics" span 4.
- **Responsiveness**: On mobile, the sidebar collapses into a bottom-bar navigation or a hamburger drawer, and the 12-column grid reflows into a single-column vertical stack.
- **Density**: A tight 4px base unit ensures that the interface feels "pro" and maximized for screen real estate.

## Elevation & Depth

Visual hierarchy is achieved through **Tonal Layering** and **Subtle Blurs** rather than traditional heavy shadows.

- **Level 0 (Base)**: The #0f172a background.
- **Level 1 (Cards)**: #1e293b surfaces with a 1px border (#334155).
- **Level 2 (Overlays/Modals)**: Semi-transparent #1e293b with a 12px backdrop-blur (Glassmorphism) to maintain context of the underlying logs while focusing on the action.
- **Borders**: All interactive elements use a low-contrast "ghost border" that glows slightly when focused or active, echoing the look of a terminal cursor.

## Shapes

The design system uses a consistent **Rounded (8px)** language to soften the industrial nature of the terminal aesthetic.

- **Standard Elements**: Cards, input fields, and buttons all use `rounded-md` (8px).
- **Small Elements**: Chips and status indicators use `rounded-sm` (4px).
- **Selection States**: Highlighting a session in the sidebar uses a rounded rectangle that does not touch the screen edge, creating a floating "pill-lite" effect.

## Components

- **Buttons**: Primary buttons are solid Slate with white text. Status-specific actions (e.g., "Deploy") use a ghost-style border in Emerald.
- **Status Chips**: Small, monospaced labels with a subtle background tint and a leading "indicator dot" (e.g., an Emerald dot for `RUNNING`).
- **Cards**: The primary container for session data. They feature a 1px border and a header section using a monospaced session ID.
- **Input Fields**: Terminal-style inputs with no fill, a simple bottom border or thin outline, and a block-style cursor animation.
- **Log Viewer**: A dedicated component with a slightly darker background than standard cards, featuring syntax highlighting for common CLI outputs and a "sticky" bottom for auto-scrolling.
- **Sidebar Nav**: High-density list items with vertical status indicators on the left edge of the active item.
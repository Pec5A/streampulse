# User training plan — StreamPulse

> Current scope: identity (register/login) and GDPR (data export/account deletion), the only features merged so far. This document will be extended as playlists (S1), live streaming (K1) and broadcasting (K2) land — see `docs/team/plan.md` for the schedule.

## Goal
Give every user profile what they need to use StreamPulse independently, including people with disabilities, without assuming any prior technical skill.

## User profiles and training needs

| Profile | What they need to learn | Format |
|---|---|---|
| **Anonymous** | What's accessible without an account, how to create one | Home screen with a clear "Create account" / "Log in" call to action |
| **Standard user** | Register, log in, manage their account (data export/deletion) | Guided in-app flow on first launch (see below) |
| **Broadcaster** | Same as standard user + (upcoming: start/stop a stream, ticket K2) | Same, completed once K2 is merged |
| **Admin** | Same as standard user + (upcoming: user management, ticket S2) | Same, completed once S2 is merged |

## Training flow — register/login (current feature)

1. **Discovery**: the home screen explains what StreamPulse is in one sentence before asking for anything.
2. **Registration**: email/username/password form, with an explicit error message if the password is under 8 characters (not just "invalid") — see `auth_handler.go`, message `"email, username and a password of at least 8 characters are required"`.
3. **Login**: email/password form, generic error on failure ("invalid credentials") — deliberately not specifying whether the email or the password is wrong, to avoid confirming to an attacker that an email exists (a security/usability trade-off made on purpose, see ADR 0001).
4. **Account management (GDPR)**: from the profile screen, two explicit actions — "Download my data" (JSON export, article 15) and "Delete my account" (article 17, immediate and irreversible, with an on-device confirmation step before submitting).

## Accessibility — adapting to a diverse audience

The mobile auth/GDPR module is built with `flutter_bloc` (ADR 0001), which keeps business logic separate from the UI — useful for the adaptations below without duplicating logic:

- **Visual impairment**: every form field (`login`/`register`) must carry an explicit `Semantics label` for screen readers (VoiceOver/TalkBack) — e.g. "Email field, required" rather than relying only on the visual placeholder. Error message contrast must meet WCAG AA (to be validated in a dedicated pass, ticket S2 — app-wide accessibility).
- **Motor impairment**: touch targets ≥ 44×44px on "Log in"/"Sign up"/"Delete account" buttons; full keyboard navigation for the web player (see ticket K1, outside the auth scope).
- **Cognitive impairment**: plain-language error messages, one action per screen (no combined register+login screen), an explicit confirmation before the irreversible account-deletion action.
- **Hearing impairment**: not applicable to auth/GDPR (no audio content in this scope) — will matter for the audio player (K1): plan visual state indicators (playing/paused/buffering) that don't depend on sound.

## What's still missing (honest scope)
- No video tutorial or step-by-step "first launch" mode implemented — only the screens themselves with clear labels/errors.
- The full accessibility pass (actual measured contrast, screen-reader testing) is ticket S2, not merged yet.
- This plan will need to be extended with the streaming (K1), broadcaster (K2), playlists (S1) and admin (S2) flows as they get merged.

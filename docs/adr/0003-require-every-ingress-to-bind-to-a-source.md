# Require every ingress to bind to a Source

Every independently configured RSS address, API, webpage, Webhook, file location, or manual inbox is a Source. Pulse will not create a hidden default Source or expose a global anonymous ingestion endpoint: users explicitly create Manual and Webhook Sources, and each Webhook Source receives an independent rotatable secret. This keeps mapping, identity, rules, diagnostics, scheduling, and credential isolation consistent across pull and push ingestion.

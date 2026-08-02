---
status: accepted
---

# Persist Source and Folder navigation order

Pulse treats navigation order as organization state. The Folder list has its own order, each Folder has an independent order for its Source memberships, and the root list has an independent order for Sources that are not assigned to any Folder. A Source may belong to multiple Folders and may therefore have a different position in each one; new items append to the end of their respective list, and removing a Source from its last Folder restores its root position or appends it when no root position exists. This keeps the existing many-to-many Folder membership model and avoids a single Source position controlling multiple projections.

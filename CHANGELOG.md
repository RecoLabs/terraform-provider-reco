## 0.1.0 (Unreleased)

FEATURES:

* **New Resource:** `reco_user`
* **New Resource:** `reco_role`
* **New Resource:** `reco_api_key`
* **New Resource:** `reco_posture_check`
* **New Resource:** `reco_threat_detection_policy`
* **New Data Source:** `reco_users`
* **New Data Source:** `reco_roles`
* **New Data Source:** `reco_api_keys`
* **New Data Source:** `reco_policies`
* **New Data Source:** `reco_posture_checks`
* **New Data Source:** `reco_integrations`
* `reco_posture_check` reads the full definition from Reco, so changes made outside Terraform are detected and import fills every attribute. It also supports `entities`, `affected_entities_preset_id` and `affected_entities_columns`.

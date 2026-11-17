# Import by the framework ID (the `id` in the Compliance Frameworks list).
# Only custom frameworks can be imported. The API cannot return a framework's
# controls, so add them to the configuration before applying any change:
# an apply replaces the controls in Reco with the configured ones.
import {
  to = reco_compliance_framework.internal_controls
  id = "00000000-0000-4000-8000-000000000001"
}

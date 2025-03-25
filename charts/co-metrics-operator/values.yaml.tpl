# Default values for co-metrics-operator.
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

replicaCount: 1

image:
  repository: deploy-releases-hyperspace-docker.common.cdn.repositories.cloud.sap/cloud-orchestration/co-metrics-operator
  pullPolicy: IfNotPresent
  # Overrides the image tag whose default is the chart appVersion.
  tag: $OPERATOR_VERSION

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  # Specifies whether a service account should be created
  create: true
  # Automatically mount a ServiceAccount's API credentials?
  automount: true
  # Annotations to add to the service account
  annotations: {}
  # The name of the service account to use.
  # If not set and create is true, a name is generated using the fullname template
  name: ""

init:
  enabled: true

  args: []
  extraArgs: []

  env: []
  # Extra environment variables to add to the init container.
  extraEnv: []

  # Volumes to mount to the init container.
  volumeMounts: []
  # Extra volumes to mount to the init container.
  extraVolumeMounts: []

manager:
  args: []
  extraArgs: []

  env: []
  # Extra environment variables to add to the manager container.
  extraEnv: []

  # Volumes to mount to the manager container.
  volumeMounts: []
  # Extra volumes to mount to the manager container.
  extraVolumeMounts: []

# Volumes to pass to pod.
volumes: []

# Extra volumes to pass to pod.
extraVolumes: []

podAnnotations: {}
podLabels: {}

podSecurityContext: {}
  # fsGroup: 2000

securityContext:
  runAsNonRoot: true
  # capabilities:
  #   drop:
  #   - ALL
  # readOnlyRootFilesystem: true
  # runAsUser: 1000

resources: {}
  # limits:
  #   cpu: 100m
  #   memory: 128Mi
  # requests:
  #   cpu: 100m
  #   memory: 128Mi

crds:
  manage: true

metrics:
  listen:
    port: 8080
  service:
    enabled: false
    port: 8080
    type: ClusterIP
    annotations: {}

webhooks:
  manage: true
  url: ""
  listen:
    port: 9443
  service:
    enabled: true
    port: 443
    type: ClusterIP
    annotations: {}

rbac:
  clusterRole:
    rules:
      - apiGroups:
          - metrics.cloud.sap
        resources:
          - managedmetrics
          - metrics
          - metrics/status
          - managedmetrics/status
        verbs:
          - "*"
      - apiGroups:
          - ""
        resources:
          - secrets
        verbs:
          - get
          - list
          - watch
      - apiGroups: [ "*" ]
        resources: [ "*" ]
        verbs: [ "get", "list", "watch" ]

  role:
    rules: []

nodeSelector: {}

tolerations: []

affinity: {}

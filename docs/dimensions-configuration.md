# Configuring Dimensions

The Metrics Operator allows you to enrich the metrics you collect with data from your Kubernetes resources. This is achieved by defining **Dimensions**. Dimensions are key-value attributes added to your metrics, which enable powerful filtering, aggregation, and analysis in your data/monitoring backend.

> **Key Behavior to Understand**
>
> Before you start, it's crucial to understand how dimensions are handled. Both `Metric` and `ManagedMetric` resources automatically add a set of **Base Dimensions** to every metric, which typically include `group`, `version`, `kind`, and `cluster`. The key difference lies in how they handle custom dimensions on top of this base.
>
> -   **`Metric`:** Behavior is **additive**.
>     -   It **always** includes the Base Dimensions (`group`, `version`, `kind`, `cluster`).
>     -   Any custom dimensions you define in the `projections` field are **added** to these base dimensions. There is no "default" mode; you either have only the base dimensions or the base dimensions plus your custom ones.
>
> -   **`ManagedMetric`:** Behavior is **conditional**.
>     -   It is designed for Crossplane and offers a special set of **Convenience Defaults**.
>     -   **If you do NOT define a `dimensions` block:** The operator exports the Base Dimensions (`cluster`, `group`, `version`, `kind`) **plus** convenience dimensions derived from the resource's status (e.g., `ready: "true"`, `synced: "true"`).
>     -   **If you define ANY custom `dimensions`:** The convenience defaults are **disabled**. The operator exports only the Base Dimension (`cluster`) **plus** your explicitly defined custom dimensions. This allows you to take full control.

---

> **Note on Naming:** Throughout this document, we will use the term **Dimension**. In some resource types like `Metric`, this field is currently named `projections`. The functionality will be consolidated and the naming will be unified to `dimensions` across all metric types in a future release.

## Dimension Structure

A Dimension is defined with the following fields:

- `name`: The name of the dimension key that will be exported.
- `fieldPath`: A [JSONPath](https://www.rfc-editor.org/rfc/rfc9535.html#name-selectors) expression to select a value from the resource.
- `type`: Specifies the data type of the value being exported. This is crucial for handling complex data. It can be:
    - `primitive` (Default): For single values like strings, numbers, or booleans.
    - `map`: For key-value objects like `metadata.labels`. The entire map is exported as a single JSON string.
    - `slice`: For arrays like `status.conditions`. The entire slice is exported as a single JSON string.

If `type` is not specified, it defaults to `primitive`. To export a map or a slice, you **must** explicitly set `type` to `map` or `slice`, respectively.

```yaml
- name: <your-dimension-name>
  fieldPath: "path.to.your.field"
  type: "primitive" # or "map", "slice"
```

## Use Cases and Examples

### 1. Exporting Primitive Values (Default)

This is the most common use case, where you want to extract a single piece of information.

#### Example: Get a Resource's Namespace and Kind

You can define multiple dimensions to capture different attributes of a resource.

```yaml
dimensions:
  - name: namespace
    fieldPath: "metadata.namespace"
    # type: "primitive" is the default and can be omitted
  - name: kind
    fieldPath: "kind"
```

### 2. Exporting Maps (type: "map")

This capability allows you to export an entire map object, such as all labels or even the entire resource, as a single JSON-formatted string.

#### Example: Export All Labels

Instead of creating a separate dimension for each label, you can export them all in one go.

```yaml
dimensions:
  - name: all-labels
    fieldPath: "metadata.labels"
    type: "map"
```

**Resulting Metric Dimension:** `all-labels: "{\"app.kubernetes.io/name\":\"my-app\",\"environment\":\"production\"}"`

#### Example: Export the Entire Resource Manifest

The special `fieldPath: "."` selects the entire resource object. This allows you to export a full snapshot of the resource, which is intended for advanced use cases with downstream processing.

```yaml
dimensions:
  - name: resource-manifest
    fieldPath: "."
    type: "map"
```

**Resulting Metric Dimension:** A JSON string containing the complete resource manifest.

### 3. Exporting Slices (type: "slice")

You can export an entire array, or a filtered subset of an array, as a single JSON-formatted string. This is common for fields like `status.conditions` in Crossplane resources.

#### Example: Export All Status Conditions

This is useful for capturing the complete state of a resource for later analysis.

```yaml
dimensions:
  - name: status-conditions
    fieldPath: "status.conditions"
    type: "slice"
```

**Resulting Metric Dimension:** (Based on a typical Crossplane resource)
`"status-conditions": "[{\"lastTransitionTime\":\"2025-10-15T09:34:50Z\",\"reason\":\"Available\",\"status\":\"True\",\"type\":\"Ready\"},{\"lastTransitionTime\":\"2026-01-17T12:27:32Z\",\"reason\":\"ReconcileSuccess\",\"status\":\"True\",\"type\":\"Synced\"}]"`

#### Example: Export Specific Conditions into Separate Dimensions

You can use filters to select specific items from a slice. The following example exports the 'Synced' and 'Ready' conditions as two separate dimensions, each containing the full condition object as a JSON string.

```yaml
dimensions:
  - name: condition-synced
    fieldPath: "status.conditions[?(@.type=='Synced')]"
    type: "slice"
  - name: condition-ready
    fieldPath: "status.conditions[?(@.type=='Ready')]"
    type: "slice"
```

**Resulting Metric Dimension for `condition-synced`:**
`"condition-synced": "[{\"lastTransitionTime\":\"2026-01-17T12:27:32Z\",\"reason\":\"ReconcileSuccess\",\"status\":\"True\",\"type\":\"Synced\"}]"`

> **Note on JSONPath Filters:** The underlying JSONPath library does not support logical operators like `&&` or `||` within a single filter expression. To extract multiple, different items from a slice, you must define a separate dimension for each, as shown in the example above.

## Intended Use: Downstream Processing

Exporting complex `map` and `slice` types is a powerful feature primarily intended for use with a downstream processing agent, such as an [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/).

These agents can receive the metric, parse the JSON string from the dimension, and perform advanced filtering, routing, modification, or aggregation before sending the data to the final data/monitoring backend. This approach allows you to keep metric definitions in the operator simple while handling complex logic externally. Without a downstream processor, a JSON string in a dimension has limited use within most monitoring platforms.

## Warning: Be Mindful of Metric Cardinality

Using dimensions, especially with `map` or `slice` types, can significantly increase metric **cardinality**. Cardinality refers to the number of unique time series generated by a metric.

If you export a dimension like `metadata.labels`, `metadata.annotations`, or a full `resource-manifest` for thousands of resources, you will create a unique time series for **each unique combination of dimension values**. High cardinality can overwhelm your monitoring backend.

**Use this feature wisely.** It is best suited for data where the number of unique combinations is manageable. Avoid exporting highly variable or unique-per-resource data as dimensions unless you have a specific need and have a downstream processing agent in place to manage the data.
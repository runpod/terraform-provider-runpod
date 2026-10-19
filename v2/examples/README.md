# Examples

Documentation snippets only: `provider/`, `resources/`, `data-sources/`. They
are the sources tfplugindocs renders into the Example Usage sections of the
repository-root `docs/`, one snippet per resource and data source, and they
carry no `versions.tf`. They are not modules, and `task validate-tf` does not
touch them.

- `provider/provider.tf`: the Example Usage on the index page, a provider
  block and one template.
- `resources/runpod_<type>/resource.tf`: the snippet on a resource page.
- `resources/runpod_<type>/import.sh`: the Import section of that page.
- `data-sources/runpod_<type>/data-source.tf`: the snippet on a data-source
  page.

Runnable configurations live in [../modules/](../modules). Start there.

# Juju Developer Induction
This is an onboarding guide for new developers on the Juju project. It will
guide you through the developer documentation to help you understand how Juju is
put together. At the end is a quiz to test your knowledge!

## Using Juju
First, make sure that you have a good idea of what Juju is and what it can do.
Start by doing the Juju tutorial, and then try building a charm with the
charming tutorial.

Make full use of the Juju documentation to get an understanding of what Juju can
do, and make sure to report any gaps not covered by the docs to the team!

> See more: [Juju tutorial](https://juju.is/docs/juju/tutorial), [Charming
tutorial (machines)](https://juju.is/docs/sdk/write-your-first-machine-charm),
[Juju documentation](https://juju.is/)

## Understanding how Juju works
Juju is made up of many components, this section will guide you through the
major ones.
### Architectural overview
See the [Architectural overview](architectural-overview.md).
### The API and facades
The Juju API is defined by the [API json
schema](../apiserver/facades/schema.json) 

TODO - write short doc about this including how to view it on the web interface. 

The `api` folder contains the
serialisation and deserialization code.

The `apiserver` folder contains the
facades that handle the requests. See [Facades](facades.md).

### Workers and the dependency engine
Workers are generally in charge of long-running processes in juju. The
dependency engine managers which workers are running.
See [Workers](dev/reference/worker.md), [The dependency engine](dev/reference/dependency-package.md)
### State (mongo and DQLite)
See [MongoDB and Consistency](MongoDB-and-Consistency.md). 

TODO find some documentation about DQLite and link here
### Cloud Providers
The cloud providers are the abstraction layer that allow juju to work on
different clouds e.g. aws, openstack, maas, ect.
[Implementing enviroment providers](implementing-environment-providers.md)

TODO Could do with more indiual documentation on every provider.
### Juju clients (cli, terraform, pylibjuju)
TODO Link to the introductions for the different clients.
[Third party Juju clients](third-party-go-clients.md)
### Contributing
See [CONTRIBUING.md](CONTRIBUTING.md) for etiquette around pull requests. The
[read before contributing guide](read-before-contributing.md) has a lot of great
tips about coding practice in Juju.

## Quiz time!
After reading the documentation above, write answers to the following questions
and send your answers to a member of the Juju team to check. This should help
fill in gaps in your knowledge.

- Describe what happens in Juju when you do `juju config mysql foo=bar`. Explain
  how it progresses from the CLI, to the controller, and eventually to the charm
  code itself.

- A “flag” worker runs when some condition is true and not when it is false. It
  loops forever and takes no action. What use does a “flag” worker have in Juju?

- Explain how facades determine which client versions are compatible with which
  controller versions.




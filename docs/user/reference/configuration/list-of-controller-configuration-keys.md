(list-of-controller-configuration-keys)=
# List of controller configuration keys

```{toctree}
:hidden:

controller-config-audit-log-exclude-methods
controller-config-juju-ha-space
controller-config-juju-mgmt-space
```

This document gives a list of all the configuration keys that can be applied to a Juju controller.
<a href="#heading--agent-logfile-max-backups"><h2 id="heading--agent-logfile-max-backups"><code>agent-logfile-max-backups</code></h2></a>

`agent-logfile-max-backups` is the maximum number of old agent log files
to keep (compressed; saved on each unit, synced to the controller).

**Type:** integer

**Default value:** 2

**Can be changed after bootstrap:** yes


<a href="#heading--agent-logfile-max-size"><h2 id="heading--agent-logfile-max-size"><code>agent-logfile-max-size</code></h2></a>

`agent-logfile-max-size` is the maximum file size of each agent log file,
in MB.

**Type:** string

**Default value:** 100M

**Can be changed after bootstrap:** yes


<a href="#heading--agent-ratelimit-max"><h2 id="heading--agent-ratelimit-max"><code>agent-ratelimit-max</code></h2></a>

`agent-ratelimit-max` is the maximum size of the token bucket used to
ratelimit the agent connections to the API server.

**Type:** integer

**Default value:** 10

**Can be changed after bootstrap:** yes


<a href="#heading--agent-ratelimit-rate"><h2 id="heading--agent-ratelimit-rate"><code>agent-ratelimit-rate</code></h2></a>

`agent-ratelimit-rate` is the interval at which a new token is added to
the token bucket, in milliseconds (ms).

**Type:** TimeDurationString

**Default value:** 250ms

**Can be changed after bootstrap:** yes


<a href="#heading--allow-model-access"><h2 id="heading--allow-model-access"><code>allow-model-access</code></h2></a>

`allow-model-access` sets whether the controller will allow users to
connect to models they have been authorized for, even when
they don't have any access rights to the controller itself.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** no


<a href="#heading--api-port"><h2 id="heading--api-port"><code>api-port</code></h2></a>

`api-port` is the port used for api connections.

**Type:** integer

**Default value:** 17070

**Can be changed after bootstrap:** no


<a href="#heading--api-port-open-delay"><h2 id="heading--api-port-open-delay"><code>api-port-open-delay</code></h2></a>

`api-port-open-delay` is a duration that the controller will wait
between when the controller has been deemed to be ready to open
the api-port and when the api-port is actually opened. This value
is only used when a controller-api-port value is set.

**Type:** TimeDurationString

**Default value:** 2s

**Can be changed after bootstrap:** yes


<a href="#heading--application-resource-download-limit"><h2 id="heading--application-resource-download-limit"><code>application-resource-download-limit</code></h2></a>

`application-resource-download-limit` limits the number of concurrent resource download
requests from unit agents which will be served. The limit is per application.
Use a value of 0 to disable the limit.

**Type:** integer

**Default value:** 0

**Can be changed after bootstrap:** yes


<a href="#heading--audit-log-capture-args"><h2 id="heading--audit-log-capture-args"><code>audit-log-capture-args</code></h2></a>

`audit-log-capture-args` determines whether the audit log will
contain the arguments passed to API methods.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


<a href="#heading--audit-log-exclude-methods"><h2 id="heading--audit-log-exclude-methods"><code>audit-log-exclude-methods</code></h2></a>

`audit-log-exclude-methods` is a list of Facade.Method names that
aren't interesting for audit logging purposes. A conversation
with only calls to these will be excluded from the
log. (They'll still appear in conversations that have other
interesting calls though.).

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--audit-log-max-backups"><h2 id="heading--audit-log-max-backups"><code>audit-log-max-backups</code></h2></a>

`audit-log-max-backups` is the number of old audit log files to keep
(compressed).

**Type:** integer

**Default value:** 10

**Can be changed after bootstrap:** yes


<a href="#heading--audit-log-max-size"><h2 id="heading--audit-log-max-size"><code>audit-log-max-size</code></h2></a>

`audit-log-max-size` is the maximum size for the current audit log
file, eg "250M".

**Type:** string

**Default value:** 300M

**Can be changed after bootstrap:** yes


<a href="#heading--auditing-enabled"><h2 id="heading--auditing-enabled"><code>auditing-enabled</code></h2></a>

`auditing-enabled` determines whether the controller will record
auditing information.

**Type:** boolean

**Default value:** true

**Can be changed after bootstrap:** yes


<a href="#heading--autocert-dns-name"><h2 id="heading--autocert-dns-name"><code>autocert-dns-name</code></h2></a>

`autocert-dns-name` sets the DNS name of the controller. If a
client connects to this name, an official certificate will be
automatically requested. Connecting to any other host name
will use the usual self-generated certificate.

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--autocert-url"><h2 id="heading--autocert-url"><code>autocert-url</code></h2></a>

`autocert-url` sets the URL used to obtain official TLS
certificates when a client connects to the API. By default,
certficates are obtains from LetsEncrypt. A good value for
testing is
"https://acme-staging.api.letsencrypt.org/directory".

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--ca-cert"><h2 id="heading--ca-cert"><code>ca-cert</code></h2></a>

`ca-cert` is the key for the controller's CA certificate attribute.

**Can be changed after bootstrap:** no


<a href="#heading--caas-image-repo"><h2 id="heading--caas-image-repo"><code>caas-image-repo</code></h2></a>

`caas-image-repo` sets the docker repo to use
for the jujud operator and mongo images.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--caas-operator-image-path"><h2 id="heading--caas-operator-image-path"><code>caas-operator-image-path</code></h2></a>
> This key is deprecated.

`caas-operator-image-path` sets the URL of the docker image
used for the application operator.
Deprecated: use `caas-image-repo`.

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--controller-api-port"><h2 id="heading--controller-api-port"><code>controller-api-port</code></h2></a>

`controller-api-port` is an optional port that may be set for controllers
that have a very heavy load. If this port is set, this port is used by
the controllers to talk to each other - used for the local API connection
as well as the pubsub forwarders. If this value is set, the api-port
isn't opened until the controllers have started properly.

**Type:** integer

**Can be changed after bootstrap:** no


<a href="#heading--controller-name"><h2 id="heading--controller-name"><code>controller-name</code></h2></a>

`controller-name` is the canonical name for the controller.

**Type:** non-empty string

**Can be changed after bootstrap:** no


<a href="#heading--controller-resource-download-limit"><h2 id="heading--controller-resource-download-limit"><code>controller-resource-download-limit</code></h2></a>

`controller-resource-download-limit` limits the number of concurrent resource download
requests from unit agents which will be served. The limit is for the combined total
of all applications on the controller.
Use a value of 0 to disable the limit.

**Type:** integer

**Default value:** 0

**Can be changed after bootstrap:** yes


<a href="#heading--controller-uuid"><h2 id="heading--controller-uuid"><code>controller-uuid</code></h2></a>

`controller-uuid` is the key for the controller UUID attribute.

**Can be changed after bootstrap:** no


<a href="#heading--features"><h2 id="heading--features"><code>features</code></h2></a>

`features` allows a list of runtime changeable features to be updated.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--identity-public-key"><h2 id="heading--identity-public-key"><code>identity-public-key</code></h2></a>

`identity-public-key` sets the public key of the identity manager.
Use this when users should be managed externally rather than
created locally on the controller.

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--identity-url"><h2 id="heading--identity-url"><code>identity-url</code></h2></a>

`identity-url` sets the URL of the identity manager.
Use this when users should be managed externally rather than
created locally on the controller.

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--juju-db-snap-channel"><h2 id="heading--juju-db-snap-channel"><code>juju-db-snap-channel</code></h2></a>

`juju-db-snap-channel` selects the channel to use when installing Mongo
snaps for focal or later. The value is ignored for older releases.

**Type:** string

**Default value:** 4.4/stable

**Can be changed after bootstrap:** no


<a href="#heading--juju-ha-space"><h2 id="heading--juju-ha-space"><code>juju-ha-space</code></h2></a>

`juju-ha-space` is the network space within which the MongoDB replica-set
should communicate.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--juju-mgmt-space"><h2 id="heading--juju-mgmt-space"><code>juju-mgmt-space</code></h2></a>

`juju-mgmt-space` is the network space that agents should use to
communicate with controllers.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--jujud-controller-snap-source"><h2 id="heading--jujud-controller-snap-source"><code>jujud-controller-snap-source</code></h2></a>

`jujud-controller-snap-source` returns the source for the controller snap.
Can be set to "legacy", "snapstore", "local" or "local-dangerous".
Cannot be changed.

**Type:** string

**Default value:** legacy

**Can be changed after bootstrap:** no


<a href="#heading--login-token-refresh-url"><h2 id="heading--login-token-refresh-url"><code>login-token-refresh-url</code></h2></a>

`login-token-refresh-url` sets the URL of the login JWT well-known endpoint.
Use this when authentication/authorisation is done using a JWT in the
login request rather than a username/password or macaroon and a local
permissions model.

**Type:** string

**Can be changed after bootstrap:** no


<a href="#heading--max-agent-state-size"><h2 id="heading--max-agent-state-size"><code>max-agent-state-size</code></h2></a>

`max-agent-state-size` is the maximum allowed size of internal state
data that agents can store to the controller in bytes. A value of 0
disables the quota checks although in principle, mongo imposes a
hard (but configurable) limit of 16M.

**Type:** integer

**Default value:** 524288

**Can be changed after bootstrap:** yes


<a href="#heading--max-charm-state-size"><h2 id="heading--max-charm-state-size"><code>max-charm-state-size</code></h2></a>

`max-charm-state-size` is the maximum allowed size of charm-specific
per-unit state data that charms can store to the controller in
bytes. A value of 0 disables the quota checks although in
principle, mongo imposes a hard (but configurable) limit of 16M.

**Type:** integer

**Default value:** 2097152

**Can be changed after bootstrap:** yes


<a href="#heading--max-debug-log-duration"><h2 id="heading--max-debug-log-duration"><code>max-debug-log-duration</code></h2></a>

`max-debug-log-duration` is used to provide a backstop to the execution of a
debug-log command. If someone starts a debug-log session in a remote
screen for example, it is very easy to disconnect from the screen while
leaving the debug-log process running. This causes unnecessary load on
the API server. The max debug-log duration has a default of 24 hours,
which should be more than enough time for a debugging session.

**Type:** TimeDurationString

**Default value:** 24h0m0s

**Can be changed after bootstrap:** yes


<a href="#heading--max-prune-txn-batch-size"><h2 id="heading--max-prune-txn-batch-size"><code>max-prune-txn-batch-size</code></h2></a>
> This key is deprecated.

`max-prune-txn-batch-size` (deprecated) is the maximum number of transactions
we will evaluate in one go when pruning. Default is 1M transactions.
A value <= 0 indicates to do all transactions at once.

**Type:** integer

**Default value:** 1000000

**Can be changed after bootstrap:** yes


<a href="#heading--max-prune-txn-passes"><h2 id="heading--max-prune-txn-passes"><code>max-prune-txn-passes</code></h2></a>
> This key is deprecated.

`max-prune-txn-passes` (deprecated) is the maximum number of batches that
we will process. So total number of transactions that can be processed
is `max-prune-txn-batch-size` * `max-prune-txn-passes`. A value <= 0 implies
'do a single pass'. If both `max-prune-txn-batch-size` and `max-prune-txn-passes`
are 0, then the default value of 1M BatchSize and 100 passes
will be used instead.

**Type:** integer

**Default value:** 100

**Can be changed after bootstrap:** yes


<a href="#heading--max-txn-log-size"><h2 id="heading--max-txn-log-size"><code>max-txn-log-size</code></h2></a>

`max-txn-log-size` is the maximum size the of capped txn log collection, eg "10M".

**Type:** string

**Default value:** 10M

**Can be changed after bootstrap:** no


<a href="#heading--migration-agent-wait-time"><h2 id="heading--migration-agent-wait-time"><code>migration-agent-wait-time</code></h2></a>

`migration-agent-wait-time` is the maximum time that the migration-master
worker will wait for agents to report for a migration phase when
executing a model migration.

**Type:** TimeDurationString

**Default value:** 15m0s

**Can be changed after bootstrap:** yes


<a href="#heading--model-logfile-max-backups"><h2 id="heading--model-logfile-max-backups"><code>model-logfile-max-backups</code></h2></a>

`model-logfile-max-backups` is the number of old model
log files to keep (compressed).

**Type:** integer

**Default value:** 2

**Can be changed after bootstrap:** yes


<a href="#heading--model-logfile-max-size"><h2 id="heading--model-logfile-max-size"><code>model-logfile-max-size</code></h2></a>

`model-logfile-max-size` is the maximum size of the log file written out by the
controller on behalf of workers running for a model.

**Type:** string

**Default value:** 10M

**Can be changed after bootstrap:** yes


<a href="#heading--mongo-memory-profile"><h2 id="heading--mongo-memory-profile"><code>mongo-memory-profile</code></h2></a>

`mongo-memory-profile` sets the memory profile for MongoDB. Valid values are:
- "low": use the least possible memory
- "default": use the default memory profile.

**Type:** string

**Default value:** default

**Can be changed after bootstrap:** yes


<a href="#heading--object-store-s3-endpoint"><h2 id="heading--object-store-s3-endpoint"><code>object-store-s3-endpoint</code></h2></a>

`object-store-s3-endpoint` is the endpoint to use for S3 object stores.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--object-store-s3-static-key"><h2 id="heading--object-store-s3-static-key"><code>object-store-s3-static-key</code></h2></a>

`object-store-s3-static-key` is the static key to use for S3 object stores.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--object-store-s3-static-secret"><h2 id="heading--object-store-s3-static-secret"><code>object-store-s3-static-secret</code></h2></a>

`object-store-s3-static-secret` is the static secret to use for S3 object
stores.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--object-store-s3-static-session"><h2 id="heading--object-store-s3-static-session"><code>object-store-s3-static-session</code></h2></a>

`object-store-s3-static-session` is the static session token to use for S3
object stores.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--object-store-type"><h2 id="heading--object-store-type"><code>object-store-type</code></h2></a>

`object-store-type` is the type of object store to use for storing blobs.
This isn't currently allowed to be changed dynamically, that will come
when we support multiple object store types (not including state).

**Type:** string

**Default value:** file

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-enabled"><h2 id="heading--open-telemetry-enabled"><code>open-telemetry-enabled</code></h2></a>

`open-telemetry-enabled` returns whether open telemetry is enabled.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-endpoint"><h2 id="heading--open-telemetry-endpoint"><code>open-telemetry-endpoint</code></h2></a>

`open-telemetry-endpoint` returns the endpoint at which the telemetry will
be pushed to.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-insecure"><h2 id="heading--open-telemetry-insecure"><code>open-telemetry-insecure</code></h2></a>

`open-telemetry-insecure` returns if the telemetry collector endpoint is
insecure or not. Useful for debug or local testing.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-sample-ratio"><h2 id="heading--open-telemetry-sample-ratio"><code>open-telemetry-sample-ratio</code></h2></a>

`open-telemetry-sample-ratio` returns the sample ratio for open telemetry.

**Type:** string

**Default value:** 0.10

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-stack-traces"><h2 id="heading--open-telemetry-stack-traces"><code>open-telemetry-stack-traces</code></h2></a>

`open-telemetry-stack-traces` return whether stack traces should be added per
span.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


<a href="#heading--open-telemetry-tail-sampling-threshold"><h2 id="heading--open-telemetry-tail-sampling-threshold"><code>open-telemetry-tail-sampling-threshold</code></h2></a>

`open-telemetry-tail-sampling-threshold` returns the tail sampling threshold
for open telemetry as a duration.

**Type:** TimeDurationString

**Default value:** 1ms

**Can be changed after bootstrap:** yes


<a href="#heading--prune-txn-query-count"><h2 id="heading--prune-txn-query-count"><code>prune-txn-query-count</code></h2></a>

`prune-txn-query-count` is the number of transactions to read in a single query.
Minimum of 10, a value of 0 will indicate to use the default value (1000).

**Type:** integer

**Default value:** 1000

**Can be changed after bootstrap:** yes


<a href="#heading--prune-txn-sleep-time"><h2 id="heading--prune-txn-sleep-time"><code>prune-txn-sleep-time</code></h2></a>

`prune-txn-sleep-time` is the amount of time to sleep between processing each
batch query. This is used to reduce load on the system, allowing other
queries to time to operate. On large controllers, processing 1000 txs
seems to take about 100ms, so a sleep time of 10ms represents a 10%
slowdown, but allows other systems to operate concurrently.
A negative number will indicate to use the default, a value of 0
indicates to not sleep at all.

**Type:** TimeDurationString

**Default value:** 10ms

**Can be changed after bootstrap:** yes


<a href="#heading--public-dns-address"><h2 id="heading--public-dns-address"><code>public-dns-address</code></h2></a>

`public-dns-address` is the public DNS address (and port) of the controller.

**Type:** string

**Can be changed after bootstrap:** yes


<a href="#heading--query-tracing-enabled"><h2 id="heading--query-tracing-enabled"><code>query-tracing-enabled</code></h2></a>

`query-tracing-enabled` returns whether query tracing is enabled. If so, any
queries which take longer than `query-tracing-threshold` will be logged.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** yes


<a href="#heading--query-tracing-threshold"><h2 id="heading--query-tracing-threshold"><code>query-tracing-threshold</code></h2></a>

`query-tracing-threshold` returns the "threshold" for query tracing. Any
queries which take longer than this value will be logged (if query tracing
is enabled). The lower the threshold, the more queries will be output. A
value of 0 means all queries will be output.

**Type:** TimeDurationString

**Default value:** 1s

**Can be changed after bootstrap:** yes


<a href="#heading--set-numa-control-policy"><h2 id="heading--set-numa-control-policy"><code>set-numa-control-policy</code></h2></a>
> This key is deprecated.

`set-numa-control-policy` (true/false) is deprecated.
Use to configure whether mongo is started with NUMA
controller policy turned on.

**Type:** boolean

**Default value:** false

**Can be changed after bootstrap:** no


<a href="#heading--state-port"><h2 id="heading--state-port"><code>state-port</code></h2></a>

`state-port` is the port used for mongo connections.

**Type:** integer

**Default value:** 37017

**Can be changed after bootstrap:** no


<a href="#heading--system-ssh-keys"><h2 id="heading--system-ssh-keys"><code>system-ssh-keys</code></h2></a>

`system-ssh-keys` returns the set of ssh keys that should be trusted by
agents of this controller regardless of the model.

**Type:** string

**Can be changed after bootstrap:** no



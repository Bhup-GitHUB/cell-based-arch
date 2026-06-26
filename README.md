# cell-based-arch

A small, runnable prototype of **cell-based architecture** — the pattern AWS uses to cap the blast radius of any failure to a single slice of customers.

> AWS doesn't run one big system. It runs many identical copies on purpose — so one bad deploy, one poison-pill request, or one hot key takes down a slice of customers instead of all of them.

The system is partitioned into independent **cells**. Each cell is a complete stack (its own app + its own database) serving a fixed subset of customers. A router hashes the partition key (`customer_id`) to exactly one cell. When a cell fails, only the customers on that cell are affected — the router does **not** fail over to another cell, because that would spread the blast radius instead of containing it.

```
blast radius = customers per cell ÷ total customers
```

More copies = more reliability. That's the surprising part.

![cell-based architecture](Screenshot%202026-06-27%20at%2012.08.30%20AM.png)

## What's here

| Component | Path | Role |
|---|---|---|
| Router | `cmd/router`, `internal/router` | Hashes `customer_id` (fnv32a mod N) to a fixed cell, forwards the request, polls cell health, contains failure |
| Cell | `cmd/cell`, `internal/cell` | A full stack — HTTP app + its own Postgres. Serves the orders API for its slice of customers |
| Deployer | `cmd/deployer`, `internal/deploy` | Cell-by-cell rolling deploy: promote → bake → health canary → proceed, or auto-rollback that one cell and stop |
| Infra | `infra/` | Dockerfiles, a `docker-compose.yml` (router + 3 cells, each with its own Postgres), and illustrative `k8s/` manifests |

## Run it

```bash
docker-compose -f infra/docker-compose.yml up -d --build
```

This brings up a router and three cells, each cell with its own Postgres. The router is published on **`localhost:18080`**; the cells on `9001`, `9002`, `9003`.

See which cell a customer is pinned to:

```bash
curl localhost:18080/whereis/cust_8992
# {"cell":"cell-2","cell_url":"http://cell2-app:9000","customer_id":"cust_8992"}
```

Place and read an order (it persists to that customer's cell, and only that cell):

```bash
curl -XPOST localhost:18080/v1/orders -d '{"customer_id":"cust_8992","item":"widget","amount":9.99}'
curl localhost:18080/v1/orders/cust_8992
```

## Demo 1 — blast radius containment

Inject a fault into cell-2 (a stand-in for a bad deploy or poison-pill request):

```bash
curl -XPOST localhost:9002/admin/fault -d '{"fail":true}'
```

Within one health-poll interval the router marks cell-2 down (`curl localhost:18080/cells`). Now compare three customers, one per cell:

```
cust_1029  (cell-1) -> HTTP 200   works
cust_8992  (cell-2) -> HTTP 503   {"cell":"cell-2","error":"cell unavailable"}
cust_4471  (cell-3) -> HTTP 200   works
```

One third of customers see an error; the rest are untouched. No failover, by design. Clear it with `curl -XPOST localhost:9002/admin/fault -d '{"fail":false}'`.

## Demo 2 — cell-by-cell deploy with auto-rollback

A healthy rollout walks the cells one at a time, baking and running a health canary between each:

```bash
go run ./cmd/deployer --version v2 --bake 2s --canary-successes 2
```

A bad rollout fails its canary on a cell and rolls **only that cell** back, then stops — the cells ahead of it are never touched:

```bash
go run ./cmd/deployer --version v3 --rollback-to v2 --fail-cell 2 --bake 2s --canary-successes 2
```

```
promoting cell1-app v3        -> canary passed
promoting cell2-app v3        -> canary FAILED  -> rolled back to v2 -> recovered
rollout summary  promoted=[cell1-app]  failed_cell=cell2-app  remaining_cells_untouched=[cell3-app]
```

## Routing

The router sorts the cell ids, hashes `fnv32a(customer_id)` and takes `mod N`, so a given customer always lands on the same cell. This prototype uses plain hash-mod-N for clarity. In production AWS goes further with **shuffle sharding** (Route 53), where each customer gets a random combination of cells so that any single noisy customer can only degrade a small, mostly-unique subset of others.

## Kubernetes

`infra/k8s/` has illustrative manifests — a namespace, a Postgres + app Deployment/Service per cell, and a router Deployment/Service — showing the same topology as the compose file for a real cluster.

## References

- AWS Well-Architected — *Reducing the Scope of Impact with Cell-Based Architecture*
- re:Invent ARC338 — *How AWS Minimizes the Blast Radius of Failures*
- AWS Route 53 — shuffle sharding
- Werner Vogels: *"Everything fails, all the time."*

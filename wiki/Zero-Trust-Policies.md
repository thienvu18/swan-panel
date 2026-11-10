# Zero-Trust Access Policies

SwanPanel implements dynamic role- and group-based access control (RBAC/GBAC) across users and network resource groups.

---

## 1. Core Abstractions

1. **Users & User Groups:**
   - Users can be grouped by team or department (e.g. `Engineers`, `Finance`, `External Contractors`).
2. **Resource Groups:**
   - Isolated network subnets representing specific VPCs, clusters, or server racks (e.g. `Production DBs: 10.60.10.0/24`, `Staging Cluster: 10.60.20.0/24`).
3. **Devices:**
   - Client endpoints registered under a specific user and assigned to a primary resource group.

---

## 2. Policy Compilation Model

The Policy Compiler dynamically reconciles the rule graph:

```text
User Group  ------->  [Access Rule: ALLOW/DENY]  ------->  Resource Group
 (Engineers)                                               (Production DBs)
```

- When an `ALLOW` rule exists between a user's group and a target resource group, the compiler injects the resource group's CIDR into the strongSwan IKEv2 traffic selectors (`local_ts`).
- When a `DENY` rule exists, the compiler injects explicit IPsec kernel DROP policies or omits the route from the client's routing proposal, effectively preventing lateral movement across corporate assets.

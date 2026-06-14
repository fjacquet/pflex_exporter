#!/usr/bin/env python3
"""Generate the PowerFlex Grafana dashboards from a shared set of builders (issue #24).

One generator -> all 16 dashboards, so a fix to a panel type (e.g. the health
state-timeline) propagates everywhere. Each dashboard keeps its existing uid / title /
templating; only the `panels` array is regenerated with the crispy/focused/logical rubric:
rows (Health -> Performance -> Capacity/Resilience -> Inventory), consistent units &
legends, state-timeline for health, tables for label-rich info.

Usage: python3 scripts/dashboards/generate.py   (run from repo root)
"""
import collections
import itertools
import json
import os

DS = {"type": "prometheus", "uid": "${datasource}"}
HEALTH_MAP = [{"type": "value", "options": {
    "0": {"text": "Healthy", "color": "green"},
    "1": {"text": "Degraded", "color": "yellow"},
    "2": {"text": "Failed", "color": "red"}}}]
HEALTH_STEPS = [{"color": "green", "value": None}, {"color": "yellow", "value": 1}, {"color": "red", "value": 2}]
PCT_THR = [{"color": "green", "value": None}, {"color": "yellow", "value": 75}, {"color": "red", "value": 90}]
RISK_THR = [{"color": "green", "value": None}, {"color": "yellow", "value": 1}, {"color": "red", "value": 5}]


class Grid:
    """Auto-stacking layout: panels flow left-to-right and wrap at width 24, so gridPos
    never overlaps. Call sec() to start a titled row, add() to place a panel."""

    def __init__(self):
        self.panels = []
        self.y = 0
        self.x = 0
        self.line_h = 0
        self._id = 0

    def _nid(self):
        self._id += 1
        return self._id

    def _newline(self):
        if self.x != 0:
            self.y += self.line_h
            self.x = 0
            self.line_h = 0

    def sec(self, title):
        self._newline()
        self.panels.append({"id": self._nid(), "type": "row", "title": title,
                            "collapsed": False, "gridPos": {"h": 1, "w": 24, "x": 0, "y": self.y}})
        self.y += 1

    def add(self, factory, w, h):
        if self.x + w > 24:
            self._newline()
        p = factory(self._nid(), self.x, self.y, w, h)
        self.panels.append(p)
        self.x += w
        self.line_h = max(self.line_h, h)

    def done(self):
        self._newline()
        return self.panels


def ts(title, exprs, unit, stack=False):
    def f(pid, x, y, w, h):
        custom = {"drawStyle": "line", "fillOpacity": 10 if not stack else 30, "showPoints": "never"}
        if stack:
            custom["stacking"] = {"mode": "normal"}
        return {"id": pid, "type": "timeseries", "title": title, "datasource": DS,
                "gridPos": {"h": h, "w": w, "x": x, "y": y},
                "fieldConfig": {"defaults": {"unit": unit, "custom": custom}, "overrides": []},
                "options": {"legend": {"displayMode": "table", "placement": "bottom", "calcs": ["lastNotNull", "max"]},
                            "tooltip": {"mode": "multi", "sort": "desc"}},
                "targets": [{"refId": chr(65 + i), "expr": e, "legendFormat": l} for i, (e, l) in enumerate(exprs)]}
    return f


def stat(title, expr, unit, mappings=None, thresholds=None):
    def f(pid, x, y, w, h):
        df = {"unit": unit}
        if mappings:
            df["mappings"] = mappings
        if thresholds:
            df["thresholds"] = {"mode": "absolute", "steps": thresholds}
        return {"id": pid, "type": "stat", "title": title, "datasource": DS,
                "gridPos": {"h": h, "w": w, "x": x, "y": y},
                "fieldConfig": {"defaults": df, "overrides": []},
                "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "colorMode": "value",
                            "graphMode": "area", "textMode": "auto"},
                "targets": [{"refId": "A", "expr": expr, "legendFormat": title}]}
    return f


def gauge(title, expr, unit, thresholds):
    def f(pid, x, y, w, h):
        return {"id": pid, "type": "gauge", "title": title, "datasource": DS,
                "gridPos": {"h": h, "w": w, "x": x, "y": y},
                "fieldConfig": {"defaults": {"unit": unit, "min": 0, "max": 100,
                                             "thresholds": {"mode": "absolute", "steps": thresholds}}, "overrides": []},
                "options": {"reduceOptions": {"calcs": ["lastNotNull"]}, "showThresholdMarkers": True},
                "targets": [{"refId": "A", "expr": expr, "legendFormat": title}]}
    return f


def state_timeline(title, exprs):
    def f(pid, x, y, w, h):
        return {"id": pid, "type": "state-timeline", "title": title, "datasource": DS,
                "gridPos": {"h": h, "w": w, "x": x, "y": y},
                "fieldConfig": {"defaults": {"custom": {"fillOpacity": 80, "lineWidth": 0},
                                             "mappings": HEALTH_MAP,
                                             "thresholds": {"mode": "absolute", "steps": HEALTH_STEPS},
                                             "color": {"mode": "thresholds"}}, "overrides": []},
                "options": {"mergeValues": True, "showValue": "never", "rowHeight": 0.9,
                            "legend": {"displayMode": "list", "placement": "bottom"}},
                "targets": [{"refId": chr(65 + i), "expr": e, "legendFormat": l} for i, (e, l) in enumerate(exprs)]}
    return f


def table(title, expr, exclude, rename, state_col="State"):
    def f(pid, x, y, w, h):
        return {"id": pid, "type": "table", "title": title, "datasource": DS,
                "gridPos": {"h": h, "w": w, "x": x, "y": y},
                "fieldConfig": {"defaults": {"custom": {"filterable": True, "align": "auto"}},
                                "overrides": [{"matcher": {"id": "byName", "options": state_col},
                                               "properties": [{"id": "mappings", "value": HEALTH_MAP},
                                                              {"id": "custom.cellOptions", "value": {"type": "color-text"}}]}]},
                "options": {"showHeader": True, "footer": {"show": False}},
                "targets": [{"refId": "A", "expr": expr, "format": "table", "instant": True}],
                "transformations": [{"id": "organize", "options": {"excludeByName": exclude, "renameByName": rename}}]}
    return f


# Per-generation metric naming profile.
GEN = {
    "gen1": {"bw": "bandwidth_kb_per_second", "bw_unit": "KBs", "lat": "latency", "lat_unit": "none",
             "iosz": "io_size_kb", "node": "pflex_sds", "node_id": "sds_id", "node_name": "sds",
             "cap_unit": "deckbytes"},
    "gen2": {"bw": "bandwidth_bytes_per_second", "bw_unit": "Bps", "lat": "latency_microseconds", "lat_unit": "µs",
             "iosz": "io_size_bytes", "node": "pflex_storagenode", "node_id": "storage_node_id",
             "node_name": "storage_node_name", "cap_unit": "bytes"},
}
CL = '{cluster=~"$cluster"}'


def perf_row(g, prefix, by, op="total", lat_op=None):
    """Standard performance row for an object: IOPS, bandwidth, latency by identity label."""
    p = GEN[g]
    panels = []
    panels.append((ts("IOPS", [(f'sum by ({by}) (pflex_{prefix}_iops{{cluster=~"$cluster", op="{op}"}})', "{{" + by + "}}")], "iops"), 8, 8))
    panels.append((ts("Bandwidth", [(f'sum by ({by}) (pflex_{prefix}_{p["bw"]}{{cluster=~"$cluster", op="{op}"}})', "{{" + by + "}}")], p["bw_unit"]), 8, 8))
    if lat_op is not None:
        panels.append((ts("Latency", [(f'avg by ({by}) (pflex_{prefix}_{p["lat"]}{{cluster=~"$cluster", op="{lat_op}"}})', "{{" + by + "}}")], p["lat_unit"]), 8, 8))
    return panels


def health_block(g, prefix, id_label, name_label, extra_state_cols=None):
    """Health stat + state-timeline + info table for an object type with _health/_info."""
    blocks = []
    blocks.append(("stat", stat("Worst Health", f"max(pflex_{prefix}_health{CL})", "short", HEALTH_MAP, HEALTH_STEPS), 6, 4))
    blocks.append(("timeline", state_timeline("Health Over Time",
                   [(f"max by ({name_label}) (pflex_{prefix}_health{CL})", "{{" + name_label + "}}")]), 18, 6))
    exclude = {"Time": True, "__name__": True, "Value": True, "job": True, "instance": True,
               "cluster_id": True, id_label: True}
    rename = {name_label: "Name", "cluster": "Cluster"}
    blocks.append(("table", table("Health Detail", f"pflex_{prefix}_info{CL}", exclude, rename), 24, 8))
    return blocks


def meta(uid, title, varlist, desc):
    tv = [{"name": "datasource", "type": "datasource", "query": "prometheus", "current": {}, "hide": 0}]
    if "cluster" in varlist:
        tv.append({"name": "cluster", "type": "query", "datasource": DS,
                   "query": "label_values(pflex_up, cluster)", "refresh": 2, "includeAll": True,
                   "multi": True, "current": {"text": "All", "value": "$__all"}})
    if "pool" in varlist:
        tv.append({"name": "pool", "type": "query", "datasource": DS,
                   "query": "label_values(pflex_storagepool_max_capacity_in_kb, storage_pool_name)",
                   "refresh": 2, "includeAll": True, "multi": True, "current": {"text": "All", "value": "$__all"}})
    return tv


def finalize(path, uid, title, varlist, desc, panels):
    d = json.load(open(path))
    d["uid"] = uid
    d["title"] = title
    d["description"] = desc
    d["templating"] = {"list": meta(uid, title, varlist, desc)}
    d["panels"] = panels
    d["schemaVersion"] = d.get("schemaVersion", 39)
    d["refresh"] = "30s"
    d["time"] = {"from": "now-6h", "to": "now"}
    d["tags"] = ["powerflex", "pflex_exporter"]
    d["version"] = int(d.get("version", 1)) + 1
    json.dump(d, open(path, "w"), indent=2)
    open(path, "a").write("\n")
    # overlap check
    boxes = [(p["gridPos"]["x"], p["gridPos"]["y"], p["gridPos"]["w"], p["gridPos"]["h"], p["title"]) for p in panels]
    def ov(a, b):
        ax, ay, aw, ah, _ = a; bx, by, bw, bh, _ = b
        return not (ax + aw <= bx or bx + bw <= ax or ay + ah <= by or by + bh <= ay)
    bad = [(a[4], b[4]) for a, b in itertools.combinations(boxes, 2) if ov(a, b)]
    types = dict(collections.Counter(p["type"] for p in panels))
    status = "OVERLAP!" if bad else "ok"
    print(f"  {os.path.basename(path):28} {status:9} panels={types}")
    if bad:
        print("    overlaps:", bad)
    return bad


def cluster_overview(g):
    p = GEN[g]
    grid = Grid()
    grid.sec("Health & Status")
    grid.add(stat("Clusters Up", f"sum(pflex_up{CL})", "short",
                  thresholds=[{"color": "red", "value": None}, {"color": "green", "value": 1}]), 6, 4)
    if g == "gen1":
        grid.add(gauge("Capacity Used %",
                       f"100 * sum by (cluster) (pflex_cluster_capacity_in_use_in_kb{CL}) / clamp_min(sum by (cluster) (pflex_cluster_max_capacity_in_kb{CL}), 1)", "percent", PCT_THR), 6, 4)
        grid.add(stat("Capacity At Risk %",
                      f"100 * sum by (cluster) (pflex_cluster_degraded_failed_capacity_in_kb{CL}) / clamp_min(sum by (cluster) (pflex_cluster_max_capacity_in_kb{CL}), 1)", "percent", thresholds=RISK_THR), 6, 4)
    else:
        grid.add(gauge("Capacity Used %",
                       f"100 * sum by (cluster) (pflex_cluster_physical_used{CL}) / clamp_min(sum by (cluster) (pflex_cluster_physical_total{CL}), 1)", "percent", PCT_THR), 6, 4)
        grid.add(stat("Data Reduction", f"max by (cluster) (pflex_cluster_data_reduction_ratio{CL})", "none"), 6, 4)
    grid.add(stat("Worst Component Health",
                  f"max(pflex_{('sds' if g=='gen1' else 'storagenode')}_health{CL} or pflex_device_health{CL} or pflex_sdc_health{CL})",
                  "short", HEALTH_MAP, HEALTH_STEPS), 6, 4)
    node = "sds" if g == "gen1" else "storagenode"
    node_name = p["node_name"]
    grid.add(state_timeline("Component Health Over Time",
             [(f"max(pflex_{node}_health{CL})", node.upper()),
              (f"max(pflex_device_health{CL})", "Device"),
              (f"max(pflex_sdc_health{CL})", "SDC")]), 24, 6)

    grid.sec("Capacity & Resilience")
    if g == "gen1":
        grid.add(ts("Capacity (Used / Max)", [
            (f"sum by (cluster) (pflex_cluster_capacity_in_use_in_kb{CL})", "used"),
            (f"sum by (cluster) (pflex_cluster_max_capacity_in_kb{CL})", "max")], "deckbytes"), 12, 8)
        grid.add(ts("Capacity At Risk (KiB)", [
            (f"sum by (cluster) (pflex_cluster_degraded_failed_capacity_in_kb{CL})", "degraded+failed"),
            (f"sum by (cluster) (pflex_cluster_failed_capacity_in_kb{CL})", "failed"),
            (f"sum by (cluster) (pflex_cluster_spare_capacity_in_kb{CL})", "spare")], "deckbytes"), 12, 8)
        grid.add(ts("Rebuild / Rebalance Remaining (KiB)", [
            (f"sum by (cluster) (pflex_cluster_bck_rebuild_capacity_in_kb{CL})", "bck rebuild"),
            (f"sum by (cluster) (pflex_cluster_fwd_rebuild_capacity_in_kb{CL})", "fwd rebuild"),
            (f"sum by (cluster) (pflex_cluster_rebalance_capacity_in_kb{CL})", "rebalance")], "deckbytes"), 12, 8)
        grid.add(ts("Snapshot Capacity (KiB)", [
            (f"sum by (cluster) (pflex_cluster_snapshot_capacity_in_kb{CL})", "snapshot"),
            (f"sum by (cluster) (pflex_cluster_snap_capacity_in_use_in_kb{CL})", "in use")], "deckbytes"), 12, 8)
    else:
        grid.add(ts("Physical Capacity (Used / Total / Free)", [
            (f"sum by (cluster) (pflex_cluster_physical_used{CL})", "used"),
            (f"sum by (cluster) (pflex_cluster_physical_total{CL})", "total"),
            (f"sum by (cluster) (pflex_cluster_physical_free{CL})", "free")], "bytes"), 12, 8)
        grid.add(ts("Logical & Efficiency", [
            (f"sum by (cluster) (pflex_cluster_logical_used{CL})", "logical used"),
            (f"sum by (cluster) (pflex_cluster_logical_provisioned{CL})", "logical provisioned")], "bytes"), 12, 8)

    grid.sec("Performance")
    grid.add(ts("Total IOPS (read + write)", [
        (f'sum by (cluster, direction) (pflex_cluster_iops{{cluster=~"$cluster", op="total"}})', "{{direction}}")], "iops"), 8, 8)
    grid.add(ts("Total Bandwidth", [
        (f'sum by (cluster, direction) (pflex_cluster_{p["bw"]}{{cluster=~"$cluster", op="total"}})', "{{direction}}")], p["bw_unit"]), 8, 8)
    lat_op = "userDataSdc" if g == "gen1" else "host"
    grid.add(ts("User IO Latency", [
        (f'avg by (cluster, direction) (pflex_cluster_{p["lat"]}{{cluster=~"$cluster", op="{lat_op}"}})', "{{direction}}")], p["lat_unit"]), 8, 8)

    if g == "gen1":
        grid.sec("Inventory")
        grid.add(stat("Volumes", f"sum by (cluster) (pflex_cluster_num_of_volumes{CL})", "short"), 8, 4)
        grid.add(stat("SDS", f"sum by (cluster) (pflex_cluster_num_of_sds{CL})", "short"), 8, 4)
        grid.add(stat("Devices", f"sum by (cluster) (pflex_cluster_num_of_devices{CL})", "short"), 8, 4)
    return grid.done()


def object_perf_dashboard(g, prefix, by, lat_op, with_health=False, id_label=None, name_label=None, capacity=None, pool_var=False):
    """A per-object dashboard: optional Health row, Performance row, optional Capacity row."""
    p = GEN[g]
    grid = Grid()
    cl = '{cluster=~"$cluster"' + (', storage_pool_name=~"$pool"' if pool_var else '') + '}'
    if with_health:
        grid.sec("Health")
        grid.add(stat("Worst Health", f"max(pflex_{prefix}_health{CL})", "short", HEALTH_MAP, HEALTH_STEPS), 6, 4)
        grid.add(state_timeline("Health Over Time",
                 [(f"max by ({name_label}) (pflex_{prefix}_health{CL})", "{{" + name_label + "}}")]), 18, 6)
        exclude = {"Time": True, "__name__": True, "Value": True, "job": True, "instance": True,
                   "cluster_id": True, id_label: True}
        rename = {name_label: "Name", "cluster": "Cluster", f"{prefix}_state": "State",
                  "device_state": "State", "mdm_connection_state": "MDM Conn",
                  "temperature_state": "Temp", "ssd_end_of_life_state": "SSD EOL", "error_state": "Error"}
        grid.add(table("Detail", f"pflex_{prefix}_info{CL}", exclude, rename), 24, 8)
    grid.sec("Performance")
    grid.add(ts("IOPS", [(f'sum by ({by}) (pflex_{prefix}_iops{{cluster=~"$cluster", op="total"}})', "{{" + by + "}}")], "iops"), 8, 8)
    grid.add(ts("Bandwidth", [(f'sum by ({by}) (pflex_{prefix}_{p["bw"]}{{cluster=~"$cluster", op="total"}})', "{{" + by + "}}")], p["bw_unit"]), 8, 8)
    if lat_op:
        grid.add(ts("Latency", [(f'avg by ({by}) (pflex_{prefix}_{p["lat"]}{{cluster=~"$cluster", op="{lat_op}"}})', "{{" + by + "}}")], p["lat_unit"]), 8, 8)
    if capacity:
        grid.sec("Capacity & Resilience")
        for title, exprs, unit in capacity:
            grid.add(ts(title, exprs, unit), 12, 8)
    return grid.done()


def build_all():
    R = "grafana"
    results = []
    # gen1
    results.append(finalize(f"{R}/gen1/01-cluster-overview.json", "pflex-cluster-overview",
        "PowerFlex - Cluster Overview", ["datasource", "cluster"],
        "Cluster health, capacity & resilience, performance, and inventory at a glance.", cluster_overview("gen1")))
    results.append(finalize(f"{R}/gen1/02-devices.json", "pflex-g1-devices", "PowerFlex Gen1 - Devices",
        ["datasource", "cluster"], "Device health/wear and performance.",
        object_perf_dashboard("gen1", "device", "device_name", None, with_health=True,
            id_label="device_id", name_label="device_name")))
    results.append(finalize(f"{R}/gen1/03-storage-pools.json", "pflex-storage-pools", "PowerFlex - Storage Pools",
        ["datasource", "cluster", "pool"], "Storage pool capacity, resilience, and performance.",
        object_perf_dashboard("gen1", "storagepool", "storage_pool_name", "target", pool_var=True, capacity=[
            ("Capacity (Used / Max) (KiB)", [
                ('sum by (storage_pool_name) (pflex_storagepool_capacity_in_use_in_kb{cluster=~"$cluster"})', "used"),
                ('sum by (storage_pool_name) (pflex_storagepool_max_capacity_in_kb{cluster=~"$cluster"})', "max")], "deckbytes"),
            ("Capacity At Risk / Rebuild (KiB)", [
                ('sum by (storage_pool_name) (pflex_storagepool_degraded_failed_capacity_in_kb{cluster=~"$cluster"})', "degraded+failed"),
                ('sum by (storage_pool_name) (pflex_storagepool_bck_rebuild_capacity_in_kb{cluster=~"$cluster"})', "bck rebuild"),
                ('sum by (storage_pool_name) (pflex_storagepool_rebalance_capacity_in_kb{cluster=~"$cluster"})', "rebalance")], "deckbytes")])))
    results.append(finalize(f"{R}/gen1/04-sdc.json", "pflex-g1-sdc", "PowerFlex Gen1 - SDC",
        ["datasource", "cluster"], "SDC (host) health and performance.",
        object_perf_dashboard("gen1", "sdc", "sdc_name", None, with_health=True,
            id_label="sdc_id", name_label="sdc_name")))
    results.append(finalize(f"{R}/gen1/05-sds.json", "pflex-g1-sds", "PowerFlex Gen1 - SDS",
        ["datasource", "cluster"], "SDS health and performance.",
        object_perf_dashboard("gen1", "sds", "sds", None, with_health=True,
            id_label="sds_id", name_label="sds")))
    results.append(finalize(f"{R}/gen1/06-volumes.json", "pflex-g1-volumes", "PowerFlex Gen1 - Volumes",
        ["datasource", "cluster"], "Volume performance and latency.",
        object_perf_dashboard("gen1", "volume", "volume_name", "userDataSdc")))
    results.append(finalize(f"{R}/gen1/07-protection-domains.json", "pflex-g1-pd", "PowerFlex Gen1 - Protection Domains",
        ["datasource", "cluster"], "Protection domain resilience, rebuild, and back-end latency.",
        object_perf_dashboard("gen1", "protectiondomain", "protection_domain_name", "target", capacity=[
            ("Capacity At Risk (KiB)", [
                ('sum by (protection_domain_name) (pflex_protectiondomain_degraded_failed_capacity_in_kb{cluster=~"$cluster"})', "degraded+failed"),
                ('sum by (protection_domain_name) (pflex_protectiondomain_failed_capacity_in_kb{cluster=~"$cluster"})', "failed")], "deckbytes"),
            ("Rebuild / Rebalance Remaining (KiB)", [
                ('sum by (protection_domain_name) (pflex_protectiondomain_bck_rebuild_capacity_in_kb{cluster=~"$cluster"})', "bck rebuild"),
                ('sum by (protection_domain_name) (pflex_protectiondomain_rebalance_capacity_in_kb{cluster=~"$cluster"})', "rebalance")], "deckbytes")])))
    # gen1 capacity dashboard: reuse cluster_overview capacity focus via dedicated grid
    cap1 = Grid()
    cap1.sec("Capacity")
    cap1.add(gauge("Capacity Used %", f"100 * sum by (cluster) (pflex_cluster_capacity_in_use_in_kb{CL}) / clamp_min(sum by (cluster) (pflex_cluster_max_capacity_in_kb{CL}), 1)", "percent", PCT_THR), 6, 8)
    cap1.add(ts("Capacity Breakdown (KiB)", [
        (f"sum by (cluster) (pflex_cluster_capacity_in_use_in_kb{CL})", "in use"),
        (f"sum by (cluster) (pflex_cluster_unused_capacity_in_kb{CL})", "unused"),
        (f"sum by (cluster) (pflex_cluster_spare_capacity_in_kb{CL})", "spare")], "deckbytes"), 18, 8)
    cap1.sec("Resilience")
    cap1.add(ts("Capacity At Risk (KiB)", [
        (f"sum by (cluster) (pflex_cluster_degraded_failed_capacity_in_kb{CL})", "degraded+failed"),
        (f"sum by (cluster) (pflex_cluster_failed_capacity_in_kb{CL})", "failed"),
        (f"sum by (cluster) (pflex_cluster_degraded_healthy_capacity_in_kb{CL})", "degraded(healthy)")], "deckbytes"), 12, 8)
    cap1.add(ts("Snapshot Capacity (KiB)", [
        (f"sum by (cluster) (pflex_cluster_snapshot_capacity_in_kb{CL})", "snapshot"),
        (f"sum by (cluster) (pflex_cluster_net_snapshot_capacity_in_kb{CL})", "net snapshot"),
        (f"sum by (cluster) (pflex_cluster_snap_capacity_in_use_in_kb{CL})", "in use")], "deckbytes"), 12, 8)
    results.append(finalize(f"{R}/gen1/08-cluster-capacity.json", "pflex-g1-capacity", "PowerFlex Gen1 - Cluster Capacity",
        ["datasource", "cluster"], "Cluster capacity breakdown and resilience.", cap1.done()))

    # gen2
    results.append(finalize(f"{R}/gen2/01-cluster-overview.json", "pflex-g2-cluster-overview",
        "PowerFlex Gen2 - Cluster Overview", ["datasource", "cluster"],
        "Gen2 cluster health, capacity, and performance at a glance.", cluster_overview("gen2")))
    # gen2 clusters-stacked: multi-cluster comparison
    st = Grid()
    st.sec("Across Clusters")
    st.add(ts("IOPS by cluster", [('sum by (cluster) (pflex_cluster_iops{op="total"})', "{{cluster}}")], "iops", stack=True), 12, 8)
    st.add(ts("Bandwidth by cluster", [('sum by (cluster) (pflex_cluster_bandwidth_bytes_per_second{op="total"})', "{{cluster}}")], "Bps", stack=True), 12, 8)
    st.add(ts("Physical Used by cluster", [("sum by (cluster) (pflex_cluster_physical_used)", "{{cluster}}")], "bytes", stack=True), 12, 8)
    st.add(ts("Latency by cluster", [('avg by (cluster) (pflex_cluster_latency_microseconds{op="host"})', "{{cluster}}")], "µs"), 12, 8)
    results.append(finalize(f"{R}/gen2/02-clusters-stacked.json", "pflex-g2-clusters-stacked",
        "PowerFlex Gen2 - Clusters (Stacked)", ["datasource"], "Compare clusters side by side.", st.done()))
    results.append(finalize(f"{R}/gen2/03-devices.json", "pflex-g2-devices", "PowerFlex Gen2 - Devices",
        ["datasource", "cluster"], "Device health/wear and performance (Gen2).",
        object_perf_dashboard("gen2", "device", "device_name", "device", with_health=True,
            id_label="device_id", name_label="device_name")))
    results.append(finalize(f"{R}/gen2/04-pools.json", "pflex-g2-pools", "PowerFlex Gen2 - Pools",
        ["datasource", "cluster", "pool"], "Gen2 storage pool capacity and performance.",
        object_perf_dashboard("gen2", "storagepool", "storage_pool_name", None, pool_var=True, capacity=[
            ("Physical (Used / Total)", [
                ('sum by (storage_pool_name) (pflex_storagepool_physical_used{cluster=~"$cluster"})', "used"),
                ('sum by (storage_pool_name) (pflex_storagepool_physical_total{cluster=~"$cluster"})', "total")], "bytes")])))
    results.append(finalize(f"{R}/gen2/05-sdc-hosts.json", "pflex-g2-sdc", "PowerFlex Gen2 - SDC / Hosts",
        ["datasource", "cluster"], "SDC (host) health and performance (Gen2).",
        object_perf_dashboard("gen2", "sdc", "sdc_name", "host", with_health=True,
            id_label="sdc_id", name_label="sdc_name")))
    # gen2 storage-node: health (node + sdt) + performance
    sn = Grid()
    sn.sec("Health")
    sn.add(stat("Worst Node Health", f"max(pflex_storagenode_health{CL})", "short", HEALTH_MAP, HEALTH_STEPS), 6, 4)
    sn.add(state_timeline("Node & SDT Health Over Time", [
        (f"max by (storage_node_name) (pflex_storagenode_health{CL})", "{{storage_node_name}}"),
        (f"max by (sdt_name) (pflex_sdt_health{CL})", "SDT {{sdt_name}}")]), 18, 6)
    sn.sec("Performance")
    sn.add(ts("IOPS", [('sum by (storage_node_name) (pflex_storagenode_iops{cluster=~"$cluster", op="total"})', "{{storage_node_name}}")], "iops"), 8, 8)
    sn.add(ts("Bandwidth", [('sum by (storage_node_name) (pflex_storagenode_bandwidth_bytes_per_second{cluster=~"$cluster", op="total"})', "{{storage_node_name}}")], "Bps"), 8, 8)
    sn.add(ts("Latency", [('avg by (storage_node_name) (pflex_storagenode_latency_microseconds{cluster=~"$cluster", op="device"})', "{{storage_node_name}}")], "µs"), 8, 8)
    results.append(finalize(f"{R}/gen2/06-storage-node.json", "pflex-g2-storage-node",
        "PowerFlex Gen2 - Storage Node", ["datasource", "cluster"], "Storage node + SDT health and performance.", sn.done()))
    results.append(finalize(f"{R}/gen2/07-volumes.json", "pflex-g2-volumes", "PowerFlex Gen2 - Volumes",
        ["datasource", "cluster"], "Volume performance and latency (Gen2).",
        object_perf_dashboard("gen2", "volume", "volume_name", "host")))
    # gen2 capacity
    cap2 = Grid()
    cap2.sec("Capacity")
    cap2.add(gauge("Capacity Used %", f"100 * sum by (cluster) (pflex_cluster_physical_used{CL}) / clamp_min(sum by (cluster) (pflex_cluster_physical_total{CL}), 1)", "percent", PCT_THR), 6, 8)
    cap2.add(ts("Physical (Used / Total / Free)", [
        (f"sum by (cluster) (pflex_cluster_physical_used{CL})", "used"),
        (f"sum by (cluster) (pflex_cluster_physical_total{CL})", "total"),
        (f"sum by (cluster) (pflex_cluster_physical_free{CL})", "free")], "bytes"), 18, 8)
    cap2.sec("Efficiency")
    cap2.add(ts("Logical (Used / Provisioned)", [
        (f"sum by (cluster) (pflex_cluster_logical_used{CL})", "logical used"),
        (f"sum by (cluster) (pflex_cluster_logical_provisioned{CL})", "logical provisioned")], "bytes"), 12, 8)
    cap2.add(ts("Reduction & Efficiency Ratios", [
        (f"max by (cluster) (pflex_cluster_data_reduction_ratio{CL})", "data reduction"),
        (f"max by (cluster) (pflex_cluster_efficiency_ratio{CL})", "efficiency")], "none"), 12, 8)
    results.append(finalize(f"{R}/gen2/08-cluster-capacity.json", "pflex-g2-capacity", "PowerFlex Gen2 - Cluster Capacity",
        ["datasource", "cluster"], "Gen2 cluster physical capacity and efficiency.", cap2.done()))
    return results


if __name__ == "__main__":
    bad = build_all()
    overlaps = [b for b in bad if b]
    print("\nOVERLAPS:" if overlaps else "\nNo overlaps. All 16 dashboards generated.")

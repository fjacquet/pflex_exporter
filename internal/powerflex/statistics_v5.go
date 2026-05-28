package powerflex

import (
	"encoding/json"

	"github.com/fjacquet/pflex_exporter/internal/models"
)

// v5 metric "kinds": derived per-second/size/latency families, or "" for a scalar gauge.
const (
	v5KindIOPS    = "iops"
	v5KindBW      = "bw"
	v5KindIOSize  = "iosize"
	v5KindLatency = "latency"
	v5KindScalar  = ""
)

// v5Mapping maps a PowerFlex v5 metric name to an exported sample shape.
// For derived kinds, Op and Direction become the op/direction labels. For scalars
// (Kind == v5KindScalar), Op is the field name used to build pflex_<obj>_<op>.
type v5Mapping struct {
	Kind      string
	Op        string
	Direction string
}

// v5Response is the /dtapi/rest/v1/metrics/query response.
type v5Response struct {
	Resources []v5Resource `json:"resources"`
}

type v5Resource struct {
	ID      string     `json:"id"`
	Metrics []v5Metric `json:"metrics"`
}

type v5Metric struct {
	Name   string    `json:"name"`
	Values []float64 `json:"values"`
}

// StatisticsV5 holds parsed v5 statistics: type -> objectID -> metricName -> value.
type StatisticsV5 struct {
	ByType map[string]map[string]map[string]float64
}

// Object returns the metric map for a given type/object id, or nil.
func (s *StatisticsV5) Object(objType, id string) map[string]float64 {
	if byID, ok := s.ByType[objType]; ok {
		return byID[id]
	}
	return nil
}

// parseV5Response converts a v5 query response into id -> {metric: value}, taking the
// first value of each metric series.
func parseV5Response(body []byte) (map[string]map[string]float64, error) {
	var resp v5Response
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]map[string]float64, len(resp.Resources))
	for _, r := range resp.Resources {
		vals := make(map[string]float64, len(r.Metrics))
		for _, m := range r.Metrics {
			if len(m.Values) > 0 {
				vals[m.Name] = m.Values[0]
			}
		}
		out[r.ID] = vals
	}
	return out, nil
}

// v5ResourceType maps normalized type names to v5 API resource_type strings.
var v5ResourceType = map[string]string{
	models.TypeSystem:           "system",
	models.TypeStorageNode:      "storage_node",
	models.TypeDevice:           "device",
	models.TypeVolume:           "volume",
	models.TypeStoragePool:      "storage_pool",
	models.TypeSdc:              "sdc",
	models.TypeDeviceGroup:      "device_group",
	models.TypeProtectionDomain: "protection_domain",
	models.TypeSdt:              "sdt",
}

// v5Metrics is the per-type metric request/mapping table, ported from Dell's
// powerflex-gen2-monitoring/siometrics.py (v5_metrics). The keys are the metrics
// requested from the v5 API; the values map each to (kind, op, direction).
var v5Metrics = map[string]map[string]v5Mapping{
	models.TypeSystem: {
		"host_read_iops":                 {v5KindIOPS, "host", "read"},
		"host_write_iops":                {v5KindIOPS, "host", "write"},
		"host_trim_iops":                 {v5KindIOPS, "host", "trim"},
		"total_device_read_iops":         {v5KindIOPS, "device", "read"},
		"total_device_write_iops":        {v5KindIOPS, "device", "write"},
		"device_local_read_iops":         {v5KindIOPS, "device_local", "read"},
		"device_local_write_iops":        {v5KindIOPS, "device_local", "write"},
		"device_remote_read_iops":        {v5KindIOPS, "device_remote", "read"},
		"device_remote_write_iops":       {v5KindIOPS, "device_remote", "write"},
		"storage_fe_read_iops":           {v5KindIOPS, "storage_fe", "read"},
		"storage_fe_write_iops":          {v5KindIOPS, "storage_fe", "write"},
		"storage_fe_trim_iops":           {v5KindIOPS, "storage_fe", "trim"},
		"host_read_bandwidth":            {v5KindBW, "host", "read"},
		"host_write_bandwidth":           {v5KindBW, "host", "write"},
		"host_trim_bandwidth":            {v5KindBW, "host", "trim"},
		"total_device_read_bandwidth":    {v5KindBW, "device", "read"},
		"total_device_write_bandwidth":   {v5KindBW, "device", "write"},
		"device_local_read_bandwidth":    {v5KindBW, "device_local", "read"},
		"device_local_write_bandwidth":   {v5KindBW, "device_local", "write"},
		"device_remote_read_bandwidth":   {v5KindBW, "device_remote", "read"},
		"device_remote_write_bandwidth":  {v5KindBW, "device_remote", "write"},
		"storage_fe_read_bandwidth":      {v5KindBW, "storage_fe", "read"},
		"storage_fe_write_bandwidth":     {v5KindBW, "storage_fe", "write"},
		"storage_fe_trim_bandwidth":      {v5KindBW, "storage_fe", "trim"},
		"avg_fe_read_io_size":            {v5KindIOSize, "host", "read"},
		"avg_fe_write_io_size":           {v5KindIOSize, "host", "write"},
		"avg_fe_trim_io_size":            {v5KindIOSize, "host", "trim"},
		"avg_device_read_io_size":        {v5KindIOSize, "device", "read"},
		"avg_device_write_io_size":       {v5KindIOSize, "device", "write"},
		"avg_host_read_latency":          {v5KindLatency, "host", "read"},
		"avg_host_write_latency":         {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":          {v5KindLatency, "host", "trim"},
		"avg_controller_to_host_latency": {v5KindLatency, "controller_to_host", ""},
		"avg_host_to_controller_latency": {v5KindLatency, "host_to_controller", ""},
		"avg_device_read_latency":        {v5KindLatency, "device", "read"},
		"avg_device_write_latency":       {v5KindLatency, "device", "write"},
		"storage_fe_read_latency":        {v5KindLatency, "storage_fe", "read"},
		"storage_fe_write_latency":       {v5KindLatency, "storage_fe", "write"},
		"storage_fe_trim_latency":        {v5KindLatency, "storage_fe", "trim"},
		"raw_total":                      {v5KindScalar, "raw_total", ""},
		"raw_used":                       {v5KindScalar, "raw_used", ""},
		"raw_free":                       {v5KindScalar, "raw_free", ""},
		"raw_system":                     {v5KindScalar, "raw_system", ""},
		"physical_total":                 {v5KindScalar, "physical_total", ""},
		"physical_used":                  {v5KindScalar, "physical_used", ""},
		"physical_free":                  {v5KindScalar, "physical_free", ""},
		"physical_system":                {v5KindScalar, "physical_system", ""},
		"logical_used":                   {v5KindScalar, "logical_used", ""},
		"logical_provisioned":            {v5KindScalar, "logical_provisioned", ""},
		"logical_owned":                  {v5KindScalar, "logical_owned", ""},
		"unreducible_data":               {v5KindScalar, "unreducible_data", ""},
		"compression_ratio":              {v5KindScalar, "compression_ratio", ""},
		"data_reduction_ratio":           {v5KindScalar, "data_reduction_ratio", ""},
		"efficiency_ratio":               {v5KindScalar, "efficiency_ratio", ""},
		"thin_provisioning_ratio":        {v5KindScalar, "thin_provisioning_ratio", ""},
		"patterns_saving_ratio":          {v5KindScalar, "patterns_saving_ratio", ""},
		"snapshot_saving_ratio":          {v5KindScalar, "snapshot_saving_ratio", ""},
		"reducible_ratio":                {v5KindScalar, "reducible_ratio", ""},
	},
	models.TypeStorageNode: {
		"total_device_read_iops":        {v5KindIOPS, "device", "read"},
		"total_device_write_iops":       {v5KindIOPS, "device", "write"},
		"device_local_read_iops":        {v5KindIOPS, "device_local", "read"},
		"device_local_write_iops":       {v5KindIOPS, "device_local", "write"},
		"device_remote_read_iops":       {v5KindIOPS, "device_remote", "read"},
		"device_remote_write_iops":      {v5KindIOPS, "device_remote", "write"},
		"storage_fe_read_iops":          {v5KindIOPS, "storage_fe", "read"},
		"storage_fe_write_iops":         {v5KindIOPS, "storage_fe", "write"},
		"storage_fe_trim_iops":          {v5KindIOPS, "storage_fe", "trim"},
		"total_device_read_bandwidth":   {v5KindBW, "device", "read"},
		"total_device_write_bandwidth":  {v5KindBW, "device", "write"},
		"device_local_read_bandwidth":   {v5KindBW, "device_local", "read"},
		"device_local_write_bandwidth":  {v5KindBW, "device_local", "write"},
		"device_remote_read_bandwidth":  {v5KindBW, "device_remote", "read"},
		"device_remote_write_bandwidth": {v5KindBW, "device_remote", "write"},
		"storage_fe_read_bandwidth":     {v5KindBW, "storage_fe", "read"},
		"storage_fe_write_bandwidth":    {v5KindBW, "storage_fe", "write"},
		"storage_fe_trim_bandwidth":     {v5KindBW, "storage_fe", "trim"},
		"avg_fe_read_io_size":           {v5KindIOSize, "host", "read"},
		"avg_fe_write_io_size":          {v5KindIOSize, "host", "write"},
		"avg_fe_trim_io_size":           {v5KindIOSize, "host", "trim"},
		"avg_device_read_io_size":       {v5KindIOSize, "device", "read"},
		"avg_device_write_io_size":      {v5KindIOSize, "device", "write"},
		"avg_device_read_latency":       {v5KindLatency, "device", "read"},
		"avg_device_write_latency":      {v5KindLatency, "device", "write"},
		"storage_fe_read_latency":       {v5KindLatency, "storage_fe", "read"},
		"storage_fe_write_latency":      {v5KindLatency, "storage_fe", "write"},
		"storage_fe_trim_latency":       {v5KindLatency, "storage_fe", "trim"},
		"raw_total":                     {v5KindScalar, "raw_total", ""},
	},
	models.TypeSdc: {
		"host_read_iops":                 {v5KindIOPS, "host", "read"},
		"host_write_iops":                {v5KindIOPS, "host", "write"},
		"host_trim_iops":                 {v5KindIOPS, "host", "trim"},
		"host_read_bandwidth":            {v5KindBW, "host", "read"},
		"host_write_bandwidth":           {v5KindBW, "host", "write"},
		"host_trim_bandwidth":            {v5KindBW, "host", "trim"},
		"avg_host_read_latency":          {v5KindLatency, "host", "read"},
		"avg_host_write_latency":         {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":          {v5KindLatency, "host", "trim"},
		"avg_controller_to_host_latency": {v5KindLatency, "controller_to_host", ""},
		"avg_host_to_controller_latency": {v5KindLatency, "host_to_controller", ""},
	},
	models.TypeVolume: {
		"host_read_iops":         {v5KindIOPS, "host", "read"},
		"host_write_iops":        {v5KindIOPS, "host", "write"},
		"host_trim_iops":         {v5KindIOPS, "host", "trim"},
		"host_read_bandwidth":    {v5KindBW, "host", "read"},
		"host_write_bandwidth":   {v5KindBW, "host", "write"},
		"host_trim_bandwidth":    {v5KindBW, "host", "trim"},
		"avg_host_read_latency":  {v5KindLatency, "host", "read"},
		"avg_host_write_latency": {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":  {v5KindLatency, "host", "trim"},
		"logical_used":           {v5KindScalar, "logical_used", ""},
		"logical_provisioned":    {v5KindScalar, "logical_provisioned", ""},
	},
	models.TypeStoragePool: {
		"host_read_iops":             {v5KindIOPS, "host", "read"},
		"host_write_iops":            {v5KindIOPS, "host", "write"},
		"host_trim_iops":             {v5KindIOPS, "host", "trim"},
		"storage_fe_read_iops":       {v5KindIOPS, "storage_fe", "read"},
		"storage_fe_write_iops":      {v5KindIOPS, "storage_fe", "write"},
		"storage_fe_trim_iops":       {v5KindIOPS, "storage_fe", "trim"},
		"host_read_bandwidth":        {v5KindBW, "host", "read"},
		"host_write_bandwidth":       {v5KindBW, "host", "write"},
		"host_trim_bandwidth":        {v5KindBW, "host", "trim"},
		"storage_fe_read_bandwidth":  {v5KindBW, "storage_fe", "read"},
		"storage_fe_write_bandwidth": {v5KindBW, "storage_fe", "write"},
		"storage_fe_trim_bandwidth":  {v5KindBW, "storage_fe", "trim"},
		"avg_fe_read_io_size":        {v5KindIOSize, "host", "read"},
		"avg_fe_write_io_size":       {v5KindIOSize, "host", "write"},
		"avg_fe_trim_io_size":        {v5KindIOSize, "host", "trim"},
		"avg_host_read_latency":      {v5KindLatency, "host", "read"},
		"avg_host_write_latency":     {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":      {v5KindLatency, "host", "trim"},
		"storage_fe_read_latency":    {v5KindLatency, "storage_fe", "read"},
		"storage_fe_write_latency":   {v5KindLatency, "storage_fe", "write"},
		"storage_fe_trim_latency":    {v5KindLatency, "storage_fe", "trim"},
		"raw_used":                   {v5KindScalar, "raw_used", ""},
		"physical_free":              {v5KindScalar, "physical_free", ""},
		"physical_total":             {v5KindScalar, "physical_total", ""},
		"physical_used":              {v5KindScalar, "physical_used", ""},
		"physical_system":            {v5KindScalar, "physical_system", ""},
		"logical_used":               {v5KindScalar, "logical_used", ""},
		"logical_provisioned":        {v5KindScalar, "logical_provisioned", ""},
		"logical_owned":              {v5KindScalar, "logical_owned", ""},
		"over_provisioning_limit":    {v5KindScalar, "over_provisioning_limit", ""},
		"unreducible_data":           {v5KindScalar, "unreducible_data", ""},
		"compression_ratio":          {v5KindScalar, "compression_ratio", ""},
		"data_reduction_ratio":       {v5KindScalar, "data_reduction_ratio", ""},
		"efficiency_ratio":           {v5KindScalar, "efficiency_ratio", ""},
		"thin_provisioning_ratio":    {v5KindScalar, "thin_provisioning_ratio", ""},
		"patterns_saving_ratio":      {v5KindScalar, "patterns_saving_ratio", ""},
		"snapshot_saving_ratio":      {v5KindScalar, "snapshot_saving_ratio", ""},
		"reducible_ratio":            {v5KindScalar, "reducible_ratio", ""},
		"utilization_ratio":          {v5KindScalar, "utilization_ratio", ""},
	},
	models.TypeDevice: {
		"total_device_read_iops":        {v5KindIOPS, "device", "read"},
		"total_device_write_iops":       {v5KindIOPS, "device", "write"},
		"device_local_read_iops":        {v5KindIOPS, "device_local", "read"},
		"device_local_write_iops":       {v5KindIOPS, "device_local", "write"},
		"device_remote_read_iops":       {v5KindIOPS, "device_remote", "read"},
		"device_remote_write_iops":      {v5KindIOPS, "device_remote", "write"},
		"total_device_read_bandwidth":   {v5KindBW, "device", "read"},
		"total_device_write_bandwidth":  {v5KindBW, "device", "write"},
		"device_local_read_bandwidth":   {v5KindBW, "device_local", "read"},
		"device_local_write_bandwidth":  {v5KindBW, "device_local", "write"},
		"device_remote_read_bandwidth":  {v5KindBW, "device_remote", "read"},
		"device_remote_write_bandwidth": {v5KindBW, "device_remote", "write"},
		"avg_device_read_io_size":       {v5KindIOSize, "device", "read"},
		"avg_device_write_io_size":      {v5KindIOSize, "device", "write"},
		"avg_device_read_latency":       {v5KindLatency, "device", "read"},
		"avg_device_write_latency":      {v5KindLatency, "device", "write"},
		"raw_total":                     {v5KindScalar, "raw_total", ""},
	},
	models.TypeProtectionDomain: {
		"host_read_iops":               {v5KindIOPS, "host", "read"},
		"host_write_iops":              {v5KindIOPS, "host", "write"},
		"host_trim_iops":               {v5KindIOPS, "host", "trim"},
		"total_device_read_iops":       {v5KindIOPS, "device", "read"},
		"total_device_write_iops":      {v5KindIOPS, "device", "write"},
		"storage_fe_read_iops":         {v5KindIOPS, "storage_fe", "read"},
		"storage_fe_write_iops":        {v5KindIOPS, "storage_fe", "write"},
		"storage_fe_trim_iops":         {v5KindIOPS, "storage_fe", "trim"},
		"host_read_bandwidth":          {v5KindBW, "host", "read"},
		"host_write_bandwidth":         {v5KindBW, "host", "write"},
		"host_trim_bandwidth":          {v5KindBW, "host", "trim"},
		"total_device_read_bandwidth":  {v5KindBW, "device", "read"},
		"total_device_write_bandwidth": {v5KindBW, "device", "write"},
		"rebalance_rate":               {v5KindBW, "rebalance", ""},
		"rebuild_rate":                 {v5KindBW, "rebuild", ""},
		"storage_fe_read_bandwidth":    {v5KindBW, "storage_fe", "read"},
		"storage_fe_write_bandwidth":   {v5KindBW, "storage_fe", "write"},
		"storage_fe_trim_bandwidth":    {v5KindBW, "storage_fe", "trim"},
		"avg_fe_read_io_size":          {v5KindIOSize, "host", "read"},
		"avg_fe_write_io_size":         {v5KindIOSize, "host", "write"},
		"avg_fe_trim_io_size":          {v5KindIOSize, "host", "trim"},
		"avg_device_read_io_size":      {v5KindIOSize, "device", "read"},
		"avg_device_write_io_size":     {v5KindIOSize, "device", "write"},
		"avg_host_read_latency":        {v5KindLatency, "host", "read"},
		"avg_host_write_latency":       {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":        {v5KindLatency, "host", "trim"},
		"avg_device_read_latency":      {v5KindLatency, "device", "read"},
		"avg_device_write_latency":     {v5KindLatency, "device", "write"},
		"storage_fe_read_latency":      {v5KindLatency, "storage_fe", "read"},
		"storage_fe_write_latency":     {v5KindLatency, "storage_fe", "write"},
		"storage_fe_trim_latency":      {v5KindLatency, "storage_fe", "trim"},
		"raw_total":                    {v5KindScalar, "raw_total", ""},
		"raw_used":                     {v5KindScalar, "raw_used", ""},
		"raw_free":                     {v5KindScalar, "raw_free", ""},
		"raw_spare":                    {v5KindScalar, "raw_spare", ""},
		"raw_system":                   {v5KindScalar, "raw_system", ""},
		"raw_spare_used":               {v5KindScalar, "raw_spare_used", ""},
		"physical_total":               {v5KindScalar, "physical_total", ""},
		"physical_used":                {v5KindScalar, "physical_used", ""},
		"physical_free":                {v5KindScalar, "physical_free", ""},
		"physical_system":              {v5KindScalar, "physical_system", ""},
		"logical_used":                 {v5KindScalar, "logical_used", ""},
		"logical_provisioned":          {v5KindScalar, "logical_provisioned", ""},
		"logical_owned":                {v5KindScalar, "logical_owned", ""},
		"unreducible_data":             {v5KindScalar, "unreducible_data", ""},
		"compression_ratio":            {v5KindScalar, "compression_ratio", ""},
		"data_reduction_ratio":         {v5KindScalar, "data_reduction_ratio", ""},
		"efficiency_ratio":             {v5KindScalar, "efficiency_ratio", ""},
		"thin_provisioning_ratio":      {v5KindScalar, "thin_provisioning_ratio", ""},
		"patterns_saving_ratio":        {v5KindScalar, "patterns_saving_ratio", ""},
		"snapshot_saving_ratio":        {v5KindScalar, "snapshot_saving_ratio", ""},
		"reducible_ratio":              {v5KindScalar, "reducible_ratio", ""},
	},
	models.TypeDeviceGroup: {
		"host_read_iops":                    {v5KindIOPS, "host", "read"},
		"host_write_iops":                   {v5KindIOPS, "host", "write"},
		"host_trim_iops":                    {v5KindIOPS, "host", "trim"},
		"total_device_read_iops":            {v5KindIOPS, "device", "read"},
		"total_device_write_iops":           {v5KindIOPS, "device", "write"},
		"device_local_read_iops":            {v5KindIOPS, "device_local", "read"},
		"device_local_write_iops":           {v5KindIOPS, "device_local", "write"},
		"device_remote_read_iops":           {v5KindIOPS, "device_remote", "read"},
		"device_remote_write_iops":          {v5KindIOPS, "device_remote", "write"},
		"storage_fe_read_iops":              {v5KindIOPS, "storage_fe", "read"},
		"storage_fe_write_iops":             {v5KindIOPS, "storage_fe", "write"},
		"storage_fe_trim_iops":              {v5KindIOPS, "storage_fe", "trim"},
		"total_device_pmem_read_iops":       {v5KindIOPS, "device_pmem", "read"},
		"total_device_pmem_write_iops":      {v5KindIOPS, "device_pmem", "write"},
		"total_wrc_read_iops":               {v5KindIOPS, "wrc", "read"},
		"total_wrc_write_iops":              {v5KindIOPS, "wrc", "write"},
		"host_read_bandwidth":               {v5KindBW, "host", "read"},
		"host_write_bandwidth":              {v5KindBW, "host", "write"},
		"host_trim_bandwidth":               {v5KindBW, "host", "trim"},
		"total_device_read_bandwidth":       {v5KindBW, "device", "read"},
		"total_device_write_bandwidth":      {v5KindBW, "device", "write"},
		"device_local_read_bandwidth":       {v5KindBW, "device_local", "read"},
		"device_local_write_bandwidth":      {v5KindBW, "device_local", "write"},
		"device_remote_read_bandwidth":      {v5KindBW, "device_remote", "read"},
		"device_remote_write_bandwidth":     {v5KindBW, "device_remote", "write"},
		"storage_fe_read_bandwidth":         {v5KindBW, "storage_fe", "read"},
		"storage_fe_write_bandwidth":        {v5KindBW, "storage_fe", "write"},
		"storage_fe_trim_bandwidth":         {v5KindBW, "storage_fe", "trim"},
		"total_device_pmem_read_bandwidth":  {v5KindBW, "device_pmem", "read"},
		"total_device_pmem_write_bandwidth": {v5KindBW, "device_pmem", "write"},
		"total_wrc_read_bandwidth":          {v5KindBW, "wrc", "read"},
		"total_wrc_write_bandwidth":         {v5KindBW, "wrc", "write"},
		"rebalance_rate":                    {v5KindBW, "rebalance", ""},
		"rebuild_rate":                      {v5KindBW, "rebuild", ""},
		"avg_fe_read_io_size":               {v5KindIOSize, "host", "read"},
		"avg_fe_write_io_size":              {v5KindIOSize, "host", "write"},
		"avg_fe_trim_io_size":               {v5KindIOSize, "host", "trim"},
		"avg_device_read_io_size":           {v5KindIOSize, "device", "read"},
		"avg_device_write_io_size":          {v5KindIOSize, "device", "write"},
		"avg_device_pmem_read_io_size":      {v5KindIOSize, "device_pmem", "read"},
		"avg_device_pmem_write_io_size":     {v5KindIOSize, "device_pmem", "write"},
		"avg_wrc_read_io_size":              {v5KindIOSize, "wrc", "read"},
		"avg_wrc_write_io_size":             {v5KindIOSize, "wrc", "write"},
		"avg_host_read_latency":             {v5KindLatency, "host", "read"},
		"avg_host_write_latency":            {v5KindLatency, "host", "write"},
		"avg_host_trim_latency":             {v5KindLatency, "host", "trim"},
		"avg_device_read_latency":           {v5KindLatency, "device", "read"},
		"avg_device_write_latency":          {v5KindLatency, "device", "write"},
		"avg_device_pmem_read_latency":      {v5KindLatency, "device_pmem", "read"},
		"avg_device_pmem_write_latency":     {v5KindLatency, "device_pmem", "write"},
		"avg_wrc_read_latency":              {v5KindLatency, "wrc", "read"},
		"avg_wrc_write_latency":             {v5KindLatency, "wrc", "write"},
		"storage_fe_read_latency":           {v5KindLatency, "storage_fe", "read"},
		"storage_fe_write_latency":          {v5KindLatency, "storage_fe", "write"},
		"storage_fe_trim_latency":           {v5KindLatency, "storage_fe", "trim"},
		"raw_total":                         {v5KindScalar, "raw_total", ""},
		"raw_used":                          {v5KindScalar, "raw_used", ""},
		"raw_free":                          {v5KindScalar, "raw_free", ""},
		"raw_spare":                         {v5KindScalar, "raw_spare", ""},
		"raw_system":                        {v5KindScalar, "raw_system", ""},
		"raw_spare_used":                    {v5KindScalar, "raw_spare_used", ""},
		"raw_rebuild":                       {v5KindScalar, "raw_rebuild", ""},
		"raw_health_degraded":               {v5KindScalar, "raw_health_degraded", ""},
		"raw_health_degraded_critical":      {v5KindScalar, "raw_health_degraded_critical", ""},
		"raw_health_failed":                 {v5KindScalar, "raw_health_failed", ""},
		"physical_total":                    {v5KindScalar, "physical_total", ""},
		"physical_used":                     {v5KindScalar, "physical_used", ""},
		"physical_free":                     {v5KindScalar, "physical_free", ""},
		"physical_system":                   {v5KindScalar, "physical_system", ""},
	},
	models.TypeSdt: {
		"avg_controller_to_host_latency": {v5KindLatency, "controller_to_host", ""},
		"avg_host_to_controller_latency": {v5KindLatency, "host_to_controller", ""},
	},
}

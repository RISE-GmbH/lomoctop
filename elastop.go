package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type ClusterStats struct {
	ClusterName string `json:"cluster_name"`
	Status      string `json:"status"`
	Indices     struct {
		Count  int `json:"count"`
		Shards struct {
			Total int `json:"total"`
		} `json:"shards"`
		Docs struct {
			Count int `json:"count"`
		} `json:"docs"`
		Store struct {
			SizeInBytes      int64 `json:"size_in_bytes"`
			TotalSizeInBytes int64 `json:"total_size_in_bytes"`
		} `json:"store"`
	} `json:"indices"`
	Nodes struct {
		Total      int `json:"total"`
		Successful int `json:"successful"`
		Failed     int `json:"failed"`
	} `json:"_nodes"`
	Process struct {
		CPU struct {
			Percent int `json:"percent"`
		} `json:"cpu"`
		OpenFileDescriptors struct {
			Min int `json:"min"`
			Max int `json:"max"`
			Avg int `json:"avg"`
		} `json:"open_file_descriptors"`
	} `json:"process"`
	Snapshots struct {
		Count int `json:"count"`
	} `json:"snapshots"`
}

type NodesInfo struct {
	Nodes map[string]struct {
		Name             string   `json:"name"`
		TransportAddress string   `json:"transport_address"`
		Version          string   `json:"version"`
		Roles            []string `json:"roles"`
		OS               struct {
			AvailableProcessors int    `json:"available_processors"`
			Name                string `json:"name"`
			Arch                string `json:"arch"`
			Version             string `json:"version"`
			PrettyName          string `json:"pretty_name"`
		} `json:"os"`
		Process struct {
			ID int `json:"id"`
		} `json:"process"`
	} `json:"nodes"`
}

type IndexStats []struct {
	Index     string `json:"index"`
	Health    string `json:"health"`
	DocsCount string `json:"docs.count"`
	StoreSize string `json:"store.size"`
	PriShards string `json:"pri"`
	Replicas  string `json:"rep"`
}

type IndexActivity struct {
	LastDocsCount    int
	InitialDocsCount int
	StartTime        time.Time
}

type IndexWriteStats struct {
	Indices map[string]struct {
		Total struct {
			Indexing struct {
				IndexTotal int64 `json:"index_total"`
			} `json:"indexing"`
		} `json:"total"`
	} `json:"indices"`
}

type ClusterHealth struct {
	ActiveShards                int     `json:"active_shards"`
	ActivePrimaryShards         int     `json:"active_primary_shards"`
	RelocatingShards            int     `json:"relocating_shards"`
	InitializingShards          int     `json:"initializing_shards"`
	UnassignedShards            int     `json:"unassigned_shards"`
	DelayedUnassignedShards     int     `json:"delayed_unassigned_shards"`
	NumberOfPendingTasks        int     `json:"number_of_pending_tasks"`
	TaskMaxWaitingTime          string  `json:"task_max_waiting_time"`
	ActiveShardsPercentAsNumber float64 `json:"active_shards_percent_as_number"`
}

type NodesStats struct {
	Nodes map[string]struct {
		Indices struct {
			Store struct {
				SizeInBytes int64 `json:"size_in_bytes"`
			} `json:"store"`
			Search struct {
				QueryTotal        int64 `json:"query_total"`
				QueryTimeInMillis int64 `json:"query_time_in_millis"`
			} `json:"search"`
			Indexing struct {
				IndexTotal        int64 `json:"index_total"`
				IndexTimeInMillis int64 `json:"index_time_in_millis"`
			} `json:"indexing"`
			Segments struct {
				Count int64 `json:"count"`
			} `json:"segments"`
		} `json:"indices"`
		OS struct {
			CPU struct {
				Percent int `json:"percent"`
			} `json:"cpu"`
			Memory struct {
				UsedInBytes  int64 `json:"used_in_bytes"`
				FreeInBytes  int64 `json:"free_in_bytes"`
				TotalInBytes int64 `json:"total_in_bytes"`
			} `json:"mem"`
			LoadAverage map[string]float64 `json:"load_average"`
		} `json:"os"`
		JVM struct {
			Memory struct {
				HeapUsedInBytes int64 `json:"heap_used_in_bytes"`
				HeapMaxInBytes  int64 `json:"heap_max_in_bytes"`
			} `json:"mem"`
			GC struct {
				Collectors struct {
					Young struct {
						CollectionCount        int64 `json:"collection_count"`
						CollectionTimeInMillis int64 `json:"collection_time_in_millis"`
					} `json:"young"`
					Old struct {
						CollectionCount        int64 `json:"collection_count"`
						CollectionTimeInMillis int64 `json:"collection_time_in_millis"`
					} `json:"old"`
				} `json:"collectors"`
			} `json:"gc"`
			UptimeInMillis int64 `json:"uptime_in_millis"`
		} `json:"jvm"`
		Transport struct {
			RxSizeInBytes int64 `json:"rx_size_in_bytes"`
			TxSizeInBytes int64 `json:"tx_size_in_bytes"`
			RxCount       int64 `json:"rx_count"`
			TxCount       int64 `json:"tx_count"`
		} `json:"transport"`
		HTTP struct {
			CurrentOpen int64 `json:"current_open"`
		} `json:"http"`
		Process struct {
			OpenFileDescriptors int64 `json:"open_file_descriptors"`
		} `json:"process"`
		FS struct {
			DiskReads  int64 `json:"disk_reads"`
			DiskWrites int64 `json:"disk_writes"`
			Total      struct {
				TotalInBytes     int64 `json:"total_in_bytes"`
				FreeInBytes      int64 `json:"free_in_bytes"`
				AvailableInBytes int64 `json:"available_in_bytes"`
			} `json:"total"`
			Data []struct {
				Path             string `json:"path"`
				TotalInBytes     int64  `json:"total_in_bytes"`
				FreeInBytes      int64  `json:"free_in_bytes"`
				AvailableInBytes int64  `json:"available_in_bytes"`
			} `json:"data"`
		} `json:"fs"`
	} `json:"nodes"`
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

var (
	latestVersion string
	versionCache  time.Time
)

var indexActivities = make(map[string]*IndexActivity)

var (
	showNodes         = true
	showRoles         = true
	showIndices       = true
	showMetrics       = true
	showHiddenIndices = false
)

var (
	header       *tview.TextView
	nodesPanel   *tview.TextView
	rolesPanel   *tview.TextView
	indicesPanel *tview.TextView
	metricsPanel *tview.TextView
)

type DataStreamResponse struct {
	DataStreams []DataStream `json:"data_streams"`
}

type DataStream struct {
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
	Template  string `json:"template"`
}

var (
	apiKey string
)

type CatNodesStats struct {
	Load1m string `json:"load_1m"`
	Name   string `json:"name"`
}

func bytesToHuman(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	units := []string{"B", "K", "M", "G", "T", "P", "E", "Z"}
	exp := 0
	val := float64(bytes)

	for val >= unit && exp < len(units)-1 {
		val /= unit
		exp++
	}

	return fmt.Sprintf("%.1f%s", val, units[exp])
}

func formatNumber(n int) string {
	str := fmt.Sprintf("%d", n)

	var result []rune
	for i, r := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, r)
	}
	return string(result)
}

func convertSizeFormat(sizeStr string) string {
	var size float64
	var unit string
	fmt.Sscanf(sizeStr, "%f%s", &size, &unit)

	unit = strings.ToUpper(strings.TrimSuffix(unit, "b"))

	return fmt.Sprintf("%d%s", int(size), unit)
}

func getPercentageColor(percent float64) string {
	switch {
	case percent < 30:
		return "green"
	case percent < 70:
		return "#00ffff" // cyan
	case percent < 85:
		return "#ffff00" // yellow
	default:
		return "#ff5555" // light red
	}
}

func getLatestVersion() string {
	// Only fetch every hour
	if time.Since(versionCache) < time.Hour && latestVersion != "" {
		return latestVersion
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/elastic/elasticsearch/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return ""
	}

	latestVersion = strings.TrimPrefix(release.TagName, "v")
	versionCache = time.Now()
	return latestVersion
}

func compareVersions(current, latest string) bool {
	if latest == "" {
		return true
	}

	// Clean up version strings
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	// Split versions into parts
	currentParts := strings.Split(current, ".")
	latestParts := strings.Split(latest, ".")

	// Compare each part
	for i := 0; i < len(currentParts) && i < len(latestParts); i++ {
		curr, _ := strconv.Atoi(currentParts[i])
		lat, _ := strconv.Atoi(latestParts[i])
		if curr != lat {
			return curr >= lat
		}
	}
	return len(currentParts) >= len(latestParts)
}

var roleColors = map[string]string{
	"master":                "#ff5555", // red
	"data":                  "#50fa7b", // green
	"data_content":          "#8be9fd", // cyan
	"data_hot":              "#ffb86c", // orange
	"data_warm":             "#bd93f9", // purple
	"data_cold":             "#f1fa8c", // yellow
	"data_frozen":           "#ff79c6", // pink
	"ingest":                "#87cefa", // light sky blue
	"ml":                    "#6272a4", // blue gray
	"remote_cluster_client": "#dda0dd", // plum
	"transform":             "#689d6a", // forest green
	"voting_only":           "#458588", // teal
	"coordinating_only":     "#d65d0e", // burnt orange
}

var legendLabels = map[string]string{
	"master":                "Master",
	"data":                  "Data",
	"data_content":          "Data Content",
	"data_hot":              "Data Hot",
	"data_warm":             "Data Warm",
	"data_cold":             "Data Cold",
	"data_frozen":           "Data Frozen",
	"ingest":                "Ingest",
	"ml":                    "Machine Learning",
	"remote_cluster_client": "Remote Cluster Client",
	"transform":             "Transform",
	"voting_only":           "Voting Only",
	"coordinating_only":     "Coordinating Only",
}

func formatNodeRoles(roles []string) string {
	// Define all possible roles and their letters in the desired order
	roleMap := map[string]string{
		"master":                "M",
		"data":                  "D",
		"data_content":          "C",
		"data_hot":              "H",
		"data_warm":             "W",
		"data_cold":             "K",
		"data_frozen":           "F",
		"ingest":                "I",
		"ml":                    "L",
		"remote_cluster_client": "R",
		"transform":             "T",
		"voting_only":           "V",
		"coordinating_only":     "O",
	}

	// Create a map of the node's roles for quick lookup
	nodeRoles := make(map[string]bool)
	for _, role := range roles {
		nodeRoles[role] = true
	}

	// Create ordered list of role keys based on their letters
	orderedRoles := []string{
		"data_content",          // C
		"data",                  // D
		"data_frozen",           // F
		"data_hot",              // H
		"ingest",                // I
		"data_cold",             // K
		"ml",                    // L
		"master",                // M
		"coordinating_only",     // O
		"remote_cluster_client", // R
		"transform",             // T
		"voting_only",           // V
		"data_warm",             // W
	}

	result := ""
	for _, role := range orderedRoles {
		letter := roleMap[role]
		if nodeRoles[role] {
			// Node has this role - use the role's color
			result += fmt.Sprintf("[%s]%s[white]", roleColors[role], letter)
		} else {
			// Node doesn't have this role - use dark grey
			result += fmt.Sprintf("[#444444]%s[white]", letter)
		}
	}

	return result
}

func getHealthColor(health string) string {
	switch health {
	case "green":
		return "green"
	case "yellow":
		return "#ffff00" // yellow
	case "red":
		return "#ff5555" // light red
	default:
		return "white"
	}
}

type indexInfo struct {
	index        string
	health       string
	docs         int
	storeSize    string
	priShards    string
	replicas     string
	writeOps     int64
	indexingRate float64
}

func updateGridLayout(grid *tview.Grid, showRoles, showIndices, showMetrics bool) {
	// Start with clean grid
	grid.Clear()

	visiblePanels := 0
	if showRoles {
		visiblePanels++
	}
	if showIndices {
		visiblePanels++
	}
	if showMetrics {
		visiblePanels++
	}

	// When only nodes panel is visible, use a single column layout
	if showNodes && visiblePanels == 0 {
		grid.SetRows(3, 0) // Header and nodes only
		grid.SetColumns(0) // Single full-width column

		// Add header and nodes panel
		grid.AddItem(header, 0, 0, 1, 1, 0, 0, false)
		grid.AddItem(nodesPanel, 1, 0, 1, 1, 0, 0, false)
		return
	}

	// Rest of the layout logic for when bottom panels are visible
	if showNodes {
		grid.SetRows(3, 0, 0) // Header, nodes, bottom panels
	} else {
		grid.SetRows(3, 0) // Just header and bottom panels
	}

	// Configure columns based on visible panels
	switch {
	case visiblePanels == 3:
		if showRoles {
			grid.SetColumns(30, -2, -1)
		}
	case visiblePanels == 2:
		if showRoles {
			grid.SetColumns(30, 0)
		} else {
			grid.SetColumns(-1, -1)
		}
	case visiblePanels == 1:
		grid.SetColumns(0)
	}

	// Always show header at top spanning all columns
	grid.AddItem(header, 0, 0, 1, visiblePanels, 0, 0, false)

	// Add nodes panel if visible, spanning all columns
	if showNodes {
		grid.AddItem(nodesPanel, 1, 0, 1, visiblePanels, 0, 0, false)
	}

	// Add bottom panels in their respective positions
	col := 0
	if showRoles {
		row := 1
		if showNodes {
			row = 2
		}
		grid.AddItem(rolesPanel, row, col, 1, 1, 0, 0, false)
		col++
	}
	if showIndices {
		row := 1
		if showNodes {
			row = 2
		}
		grid.AddItem(indicesPanel, row, col, 1, 1, 0, 0, false)
		col++
	}
	if showMetrics {
		row := 1
		if showNodes {
			row = 2
		}
		grid.AddItem(metricsPanel, row, col, 1, 1, 0, 0, false)
	}
}

func main() {
	host := flag.String("host", "http://localhost", "Elasticsearch host URL (e.g., http://localhost or https://example.com)")
	port := flag.Int("port", 9200, "Elasticsearch port")
	user := flag.String("user", os.Getenv("ES_USER"), "Elasticsearch username")
	password := flag.String("password", os.Getenv("ES_PASSWORD"), "Elasticsearch password")
	flag.StringVar(&apiKey, "apikey", os.Getenv("ES_API_KEY"), "Elasticsearch API key")

	// Add new certificate-related flags
	certFile := flag.String("cert", "", "Path to client certificate file")
	keyFile := flag.String("key", "", "Path to client private key file")
	caFile := flag.String("ca", "", "Path to CA certificate file")
	skipVerify := flag.Bool("insecure", false, "Skip TLS certificate verification")

	flag.Parse()

	// Validate and process the host URL
	if !strings.HasPrefix(*host, "http://") && !strings.HasPrefix(*host, "https://") {
		fmt.Fprintf(os.Stderr, "Error: host must start with http:// or https://\n")
		os.Exit(1)
	}

	// Validate authentication methods - only one should be used
	authMethods := 0
	if apiKey != "" {
		authMethods++
	}
	if *user != "" || *password != "" {
		authMethods++
	}
	if *certFile != "" || *keyFile != "" {
		authMethods++
	}
	if authMethods > 1 {
		fmt.Fprintf(os.Stderr, "Error: Cannot use multiple authentication methods simultaneously (API key, username/password, or certificates)\n")
		os.Exit(1)
	}

	// Validate certificate files if specified
	if (*certFile != "" && *keyFile == "") || (*certFile == "" && *keyFile != "") {
		fmt.Fprintf(os.Stderr, "Error: Both certificate and key files must be specified together\n")
		os.Exit(1)
	}

	// Strip any trailing slash from the host
	*host = strings.TrimRight(*host, "/")

	// Create TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: *skipVerify,
	}

	// Load client certificates if specified
	if *certFile != "" && *keyFile != "" {
		cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading client certificates: %v\n", err)
			os.Exit(1)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate if specified
	if *caFile != "" {
		caCert, err := os.ReadFile(*caFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading CA certificate: %v\n", err)
			os.Exit(1)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			fmt.Fprintf(os.Stderr, "Error parsing CA certificate\n")
			os.Exit(1)
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Create custom HTTP client with SSL configuration
	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 10,
	}

	app := tview.NewApplication()

	// Update the grid layout to use proportional columns
	grid := tview.NewGrid().
		SetRows(3, 0, 0).       // Three rows: header, nodes, bottom panels
		SetColumns(-1, -2, -1). // Three columns for bottom row: roles (1), indices (2), metrics (1)
		SetBorders(true)

	// Initialize the panels (move initialization to package level)
	header = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)

	nodesPanel = tview.NewTextView().
		SetDynamicColors(true)

	rolesPanel = tview.NewTextView(). // New panel for roles
						SetDynamicColors(true)

	indicesPanel = tview.NewTextView().
		SetDynamicColors(true)

	metricsPanel = tview.NewTextView().
		SetDynamicColors(true)

	// Initial layout
	updateGridLayout(grid, showRoles, showIndices, showMetrics)

	// Add panels to grid
	grid.AddItem(header, 0, 0, 1, 3, 0, 0, false). // Header spans all columns
							AddItem(nodesPanel, 1, 0, 1, 3, 0, 0, false).   // Nodes panel spans all columns
							AddItem(rolesPanel, 2, 0, 1, 1, 0, 0, false).   // Roles panel in left column
							AddItem(indicesPanel, 2, 1, 1, 1, 0, 0, false). // Indices panel in middle column
							AddItem(metricsPanel, 2, 2, 1, 1, 0, 0, false)  // Metrics panel in right column

	// Update function
	update := func() {
		baseURL := fmt.Sprintf("%s:%d", *host, *port)

		// Helper function for ES requests
		makeRequest := func(path string, target interface{}) error {
			req, err := http.NewRequest("GET", baseURL+path, nil)
			if err != nil {
				return err
			}

			// Set authentication
			if apiKey != "" {
				req.Header.Set("Authorization", fmt.Sprintf("ApiKey %s", apiKey))
			} else if *user != "" && *password != "" {
				req.SetBasicAuth(*user, *password)
			}

			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			return json.Unmarshal(body, target)
		}

		// Get cluster stats
		var clusterStats ClusterStats
		if err := makeRequest("/_cluster/stats", &clusterStats); err != nil {
			header.SetText(fmt.Sprintf("[red]Error: %v", err))
			return
		}

		// Get nodes info
		var nodesInfo NodesInfo
		if err := makeRequest("/_nodes", &nodesInfo); err != nil {
			nodesPanel.SetText(fmt.Sprintf("[red]Error: %v", err))
			return
		}

		// Get indices stats
		var indicesStats IndexStats
		if err := makeRequest("/_cat/indices?format=json", &indicesStats); err != nil {
			indicesPanel.SetText(fmt.Sprintf("[red]Error: %v", err))
			return
		}

		// Get cluster health
		var clusterHealth ClusterHealth
		if err := makeRequest("/_cluster/health", &clusterHealth); err != nil {
			indicesPanel.SetText(fmt.Sprintf("[red]Error: %v", err))
			return
		}

		// Get nodes stats
		var nodesStats NodesStats
		if err := makeRequest("/_nodes/stats", &nodesStats); err != nil {
			indicesPanel.SetText(fmt.Sprintf("[red]Error: %v", err))
			return
		}

		// Get index write stats
		var indexWriteStats IndexWriteStats
		if err := makeRequest("/_stats", &indexWriteStats); err != nil {
			indicesPanel.SetText(fmt.Sprintf("[red]Error getting write stats: %v", err))
			return
		}

		// Query and indexing metrics
		var (
			totalQueries   int64
			totalQueryTime int64
			totalIndexing  int64
			totalIndexTime int64
			totalSegments  int64
		)

		for _, node := range nodesStats.Nodes {
			totalQueries += node.Indices.Search.QueryTotal
			totalQueryTime += node.Indices.Search.QueryTimeInMillis
			totalIndexing += node.Indices.Indexing.IndexTotal
			totalIndexTime += node.Indices.Indexing.IndexTimeInMillis
			totalSegments += node.Indices.Segments.Count
		}

		queryRate := float64(totalQueries) / float64(totalQueryTime) * 1000  // queries per second
		indexRate := float64(totalIndexing) / float64(totalIndexTime) * 1000 // docs per second

		// GC metrics
		var (
			totalGCCollections int64
			totalGCTime        int64
		)

		for _, node := range nodesStats.Nodes {
			totalGCCollections += node.JVM.GC.Collectors.Young.CollectionCount + node.JVM.GC.Collectors.Old.CollectionCount
			totalGCTime += node.JVM.GC.Collectors.Young.CollectionTimeInMillis + node.JVM.GC.Collectors.Old.CollectionTimeInMillis
		}

		// Update header
		statusColor := map[string]string{
			"green":  "green",
			"yellow": "yellow",
			"red":    "red",
		}[clusterStats.Status]

		// Get max lengths after fetching node and index info
		maxNodeNameLen, maxIndexNameLen, maxTransportLen, maxIngestedLen := getMaxLengths(nodesInfo, indicesStats)

		// Update header with dynamic padding
		header.Clear()
		latestVer := getLatestVersion()
		padding := 0
		if maxNodeNameLen > len(clusterStats.ClusterName) {
			padding = maxNodeNameLen - len(clusterStats.ClusterName)
		}
		fmt.Fprintf(header, "[#00ffff]Cluster :[white] %s [#666666]([%s]%s[-]%s[#666666]) [#00ffff]Latest: [white]%s\n",
			clusterStats.ClusterName,
			statusColor,
			strings.ToUpper(clusterStats.Status),
			strings.Repeat(" ", padding),
			latestVer)
		fmt.Fprintf(header, "[#00ffff]Nodes   :[white] %d Total, [green]%d[white] Successful, [#ff5555]%d[white] Failed\n",
			clusterStats.Nodes.Total,
			clusterStats.Nodes.Successful,
			clusterStats.Nodes.Failed)
		fmt.Fprintf(header, "[#666666]Press 2-5 to toggle panels, 'h' to toggle hidden indices, 'q' to quit[white]\n")

		// Update nodes panel with dynamic width
		nodesPanel.Clear()
		fmt.Fprintf(nodesPanel, "[::b][#00ffff][[#ff5555]2[#00ffff]] Nodes Information[::-]\n\n")
		fmt.Fprint(nodesPanel, getNodesPanelHeader(maxNodeNameLen, maxTransportLen))

		// Create a sorted slice of node IDs based on node names
		var nodeIDs []string
		for id := range nodesInfo.Nodes {
			nodeIDs = append(nodeIDs, id)
		}
		sort.Slice(nodeIDs, func(i, j int) bool {
			return nodesInfo.Nodes[nodeIDs[i]].Name < nodesInfo.Nodes[nodeIDs[j]].Name
		})

		// Update node entries with dynamic width
		for _, id := range nodeIDs {
			nodeInfo := nodesInfo.Nodes[id]
			nodeStats, exists := nodesStats.Nodes[id]
			if !exists {
				continue
			}

			// Calculate resource percentages and format memory values
			cpuPercent := nodeStats.OS.CPU.Percent
			memPercent := float64(nodeStats.OS.Memory.UsedInBytes) / float64(nodeStats.OS.Memory.TotalInBytes) * 100
			heapPercent := float64(nodeStats.JVM.Memory.HeapUsedInBytes) / float64(nodeStats.JVM.Memory.HeapMaxInBytes) * 100

			// Calculate disk usage - use the data path stats
			diskTotal := int64(0)
			diskAvailable := int64(0)
			if len(nodeStats.FS.Data) > 0 {
				// Use the first data path's stats - this is the Elasticsearch data directory
				diskTotal = nodeStats.FS.Data[0].TotalInBytes
				diskAvailable = nodeStats.FS.Data[0].AvailableInBytes
			} else {
				// Fallback to total stats if data path stats aren't available
				diskTotal = nodeStats.FS.Total.TotalInBytes
				diskAvailable = nodeStats.FS.Total.AvailableInBytes
			}
			diskUsed := diskTotal - diskAvailable
			diskPercent := float64(diskUsed) / float64(diskTotal) * 100

			versionColor := "yellow"
			if compareVersions(nodeInfo.Version, latestVer) {
				versionColor = "green"
			}

			// Add this request before the nodes panel update
			var catNodesStats []CatNodesStats
			if err := makeRequest("/_cat/nodes?format=json&h=name,load_1m", &catNodesStats); err != nil {
				nodesPanel.SetText(fmt.Sprintf("[red]Error getting cat nodes stats: %v", err))
				return
			}

			// Create a map for quick lookup of load averages by node name
			nodeLoads := make(map[string]string)
			for _, node := range catNodesStats {
				nodeLoads[node.Name] = node.Load1m
			}

			fmt.Fprintf(nodesPanel, "[#5555ff]%-*s [white] [#444444]│[white] %s [#444444]│[white] [white]%*s[white] [#444444]│[white] [%s]%-7s[white] [#444444]│[white] [%s]%3d%% [#444444](%d)[white] [#444444]│[white] %4s / %4s [%s]%3d%%[white] [#444444]│[white] %4s / %4s [%s]%3d%%[white] [#444444]│[white] %4s / %4s [%s]%3d%%[white] [#444444]│[white] %-8s[white] [#444444]│[white] %s [#bd93f9]%s[white] [#444444](%s)[white]\n",
				maxNodeNameLen,
				nodeInfo.Name,
				formatNodeRoles(nodeInfo.Roles),
				maxTransportLen,
				nodeInfo.TransportAddress,
				versionColor,
				nodeInfo.Version,
				getPercentageColor(float64(cpuPercent)),
				cpuPercent,
				nodeInfo.OS.AvailableProcessors,
				formatResourceSize(nodeStats.OS.Memory.UsedInBytes),
				formatResourceSize(nodeStats.OS.Memory.TotalInBytes),
				getPercentageColor(memPercent),
				int(memPercent),
				formatResourceSize(nodeStats.JVM.Memory.HeapUsedInBytes),
				formatResourceSize(nodeStats.JVM.Memory.HeapMaxInBytes),
				getPercentageColor(heapPercent),
				int(heapPercent),
				formatResourceSize(diskUsed),
				formatResourceSize(diskTotal),
				getPercentageColor(diskPercent),
				int(diskPercent),
				formatUptime(nodeStats.JVM.UptimeInMillis),
				nodeInfo.OS.PrettyName,
				nodeInfo.OS.Version,
				nodeInfo.OS.Arch)
		}

		// Get data streams info
		var dataStreamResp DataStreamResponse
		if err := makeRequest("/_data_stream", &dataStreamResp); err != nil {
			indicesPanel.SetText(fmt.Sprintf("[red]Error getting data streams: %v", err))
			return
		}

		// Update indices panel with dynamic width
		indicesPanel.Clear()
		fmt.Fprintf(indicesPanel, "[::b][#00ffff][[#ff5555]4[#00ffff]] Indices Information[::-]\n\n")
		fmt.Fprint(indicesPanel, getIndicesPanelHeader(maxIndexNameLen, maxIngestedLen))

		// Update index entries with dynamic width
		var indices []indexInfo
		var totalDocs int
		var totalSize int64

		// Collect index information
		for _, index := range indicesStats {
			// Skip hidden indices unless showHiddenIndices is true
			if (!showHiddenIndices && strings.HasPrefix(index.Index, ".")) || index.DocsCount == "0" {
				continue
			}
			docs := 0
			fmt.Sscanf(index.DocsCount, "%d", &docs)
			totalDocs += docs

			// Track document changes
			activity, exists := indexActivities[index.Index]
			if !exists {
				indexActivities[index.Index] = &IndexActivity{
					LastDocsCount:    docs,
					InitialDocsCount: docs,
					StartTime:        time.Now(),
				}
			} else {
				activity.LastDocsCount = docs
			}

			// Get write operations count and calculate rate
			writeOps := int64(0)
			indexingRate := float64(0)
			if stats, exists := indexWriteStats.Indices[index.Index]; exists {
				writeOps = stats.Total.Indexing.IndexTotal
				if activity, ok := indexActivities[index.Index]; ok {
					timeDiff := time.Since(activity.StartTime).Seconds()
					if timeDiff > 0 {
						indexingRate = float64(docs-activity.InitialDocsCount) / timeDiff
					}
				}
			}

			indices = append(indices, indexInfo{
				index:        index.Index,
				health:       index.Health,
				docs:         docs,
				storeSize:    index.StoreSize,
				priShards:    index.PriShards,
				replicas:     index.Replicas,
				writeOps:     writeOps,
				indexingRate: indexingRate,
			})
		}

		// Calculate total size
		for _, node := range nodesStats.Nodes {
			totalSize += node.FS.Total.TotalInBytes - node.FS.Total.AvailableInBytes
		}

		// Sort indices - active ones first, then alphabetically within each group
		sort.Slice(indices, func(i, j int) bool {
			// If one is active and the other isn't, active goes first
			if (indices[i].indexingRate > 0) != (indices[j].indexingRate > 0) {
				return indices[i].indexingRate > 0
			}
			// Within the same group (both active or both inactive), sort alphabetically
			return indices[i].index < indices[j].index
		})

		// Update index entries with dynamic width
		for _, idx := range indices {
			writeIcon := "[#444444]⚪"
			if idx.indexingRate > 0 {
				writeIcon = "[#5555ff]⚫"
			}

			// Add data stream indicator
			streamIndicator := " "
			if isDataStream(idx.index, dataStreamResp) {
				streamIndicator = "[#bd93f9]⚫[white]"
			}

			// Calculate document changes with dynamic padding
			activity := indexActivities[idx.index]
			ingestedStr := ""
			if activity != nil && activity.InitialDocsCount < idx.docs {
				docChange := idx.docs - activity.InitialDocsCount
				ingestedStr = fmt.Sprintf("[green]%-*s", maxIngestedLen, fmt.Sprintf("+%s", formatNumber(docChange)))
			} else {
				ingestedStr = fmt.Sprintf("%-*s", maxIngestedLen, "")
			}

			// Format indexing rate
			rateStr := ""
			if idx.indexingRate > 0 {
				if idx.indexingRate >= 1000 {
					rateStr = fmt.Sprintf("[#50fa7b]%.1fk/s", idx.indexingRate/1000)
				} else {
					rateStr = fmt.Sprintf("[#50fa7b]%.1f/s", idx.indexingRate)
				}
			} else {
				rateStr = "[#444444]0/s"
			}

			// Convert the size format before display
			sizeStr := convertSizeFormat(idx.storeSize)

			fmt.Fprintf(indicesPanel, "%s %s[%s]%-*s[white] [#444444]│[white] %13s [#444444]│[white] %5s [#444444]│[white] %6s [#444444]│[white] %8s [#444444]│[white] %-*s [#444444]│[white] %-8s\n",
				writeIcon,
				streamIndicator,
				getHealthColor(idx.health),
				maxIndexNameLen,
				idx.index,
				formatNumber(idx.docs),
				sizeStr,
				idx.priShards,
				idx.replicas,
				maxIngestedLen,
				ingestedStr,
				rateStr)
		}

		// Calculate total indexing rate for the cluster
		totalIndexingRate := float64(0)
		for _, idx := range indices {
			totalIndexingRate += idx.indexingRate
		}

		// Format cluster indexing rate
		clusterRateStr := ""
		if totalIndexingRate > 0 {
			if totalIndexingRate >= 1000000 {
				clusterRateStr = fmt.Sprintf("[#50fa7b]%.1fM/s", totalIndexingRate/1000000)
			} else if totalIndexingRate >= 1000 {
				clusterRateStr = fmt.Sprintf("[#50fa7b]%.1fK/s", totalIndexingRate/1000)
			} else {
				clusterRateStr = fmt.Sprintf("[#50fa7b]%.1f/s", totalIndexingRate)
			}
		} else {
			clusterRateStr = "[#444444]0/s"
		}

		// Display the totals with indexing rate
		fmt.Fprintf(indicesPanel, "\n[#00ffff]Total Documents:[white] %s, [#00ffff]Total Size:[white] %s, [#00ffff]Indexing Rate:[white] %s\n",
			formatNumber(totalDocs),
			bytesToHuman(totalSize),
			clusterRateStr)

		// Move shard stats to bottom of indices panel
		fmt.Fprintf(indicesPanel, "\n[#00ffff]Shard Status:[white] Active: %d (%.1f%%), Primary: %d, Relocating: %d, Initializing: %d, Unassigned: %d\n",
			clusterHealth.ActiveShards,
			clusterHealth.ActiveShardsPercentAsNumber,
			clusterHealth.ActivePrimaryShards,
			clusterHealth.RelocatingShards,
			clusterHealth.InitializingShards,
			clusterHealth.UnassignedShards)

		// Update metrics panel
		metricsPanel.Clear()
		fmt.Fprintf(metricsPanel, "[::b][#00ffff][[#ff5555]5[#00ffff]] Cluster Metrics[::-]\n\n")

		// Define metrics keys with proper grouping
		metricKeys := []string{
			// System metrics
			"CPU",
			"Memory",
			"Heap",
			"Disk",

			// Network metrics
			"Network TX",
			"Network RX",
			"HTTP Connections",

			// Performance metrics
			"Query Rate",
			"Index Rate",

			// Miscellaneous
			"Snapshots",
		}

		// Find the longest key for proper alignment
		maxKeyLength := 0
		for _, key := range metricKeys {
			if len(key) > maxKeyLength {
				maxKeyLength = len(key)
			}
		}

		// Add padding for better visual separation
		maxKeyLength += 2

		// Helper function for metric lines with proper alignment
		formatMetric := func(name string, value string) string {
			return fmt.Sprintf("[#00ffff]%-*s[white] %s\n", maxKeyLength, name+":", value)
		}

		// CPU metrics
		totalProcessors := 0
		for _, node := range nodesInfo.Nodes {
			totalProcessors += node.OS.AvailableProcessors
		}
		cpuPercent := float64(clusterStats.Process.CPU.Percent)
		fmt.Fprint(metricsPanel, formatMetric("CPU", fmt.Sprintf("%7.1f%% [#444444](%d processors)[white]", cpuPercent, totalProcessors)))

		// Disk metrics
		diskUsed := getTotalSize(nodesStats)
		diskTotal := getTotalDiskSpace(nodesStats)
		diskPercent := float64(diskUsed) / float64(diskTotal) * 100
		fmt.Fprint(metricsPanel, formatMetric("Disk", fmt.Sprintf("%8s / %8s [%s]%5.1f%%[white]",
			bytesToHuman(diskUsed),
			bytesToHuman(diskTotal),
			getPercentageColor(diskPercent),
			diskPercent)))

		// Calculate heap and memory totals
		var (
			totalHeapUsed    int64
			totalHeapMax     int64
			totalMemoryUsed  int64
			totalMemoryTotal int64
		)

		for _, node := range nodesStats.Nodes {
			totalHeapUsed += node.JVM.Memory.HeapUsedInBytes
			totalHeapMax += node.JVM.Memory.HeapMaxInBytes
			totalMemoryUsed += node.OS.Memory.UsedInBytes
			totalMemoryTotal += node.OS.Memory.TotalInBytes
		}

		// Heap metrics
		heapPercent := float64(totalHeapUsed) / float64(totalHeapMax) * 100
		fmt.Fprint(metricsPanel, formatMetric("Heap", fmt.Sprintf("%8s / %8s [%s]%5.1f%%[white]",
			bytesToHuman(totalHeapUsed),
			bytesToHuman(totalHeapMax),
			getPercentageColor(heapPercent),
			heapPercent)))

		// Memory metrics
		memoryPercent := float64(totalMemoryUsed) / float64(totalMemoryTotal) * 100
		fmt.Fprint(metricsPanel, formatMetric("Memory", fmt.Sprintf("%8s / %8s [%s]%5.1f%%[white]",
			bytesToHuman(totalMemoryUsed),
			bytesToHuman(totalMemoryTotal),
			getPercentageColor(memoryPercent),
			memoryPercent)))

		// Network metrics
		fmt.Fprint(metricsPanel, formatMetric("Network TX", fmt.Sprintf(" %7s", bytesToHuman(getTotalNetworkTX(nodesStats)))))
		fmt.Fprint(metricsPanel, formatMetric("Network RX", fmt.Sprintf(" %7s", bytesToHuman(getTotalNetworkRX(nodesStats)))))

		// HTTP Connections and Shard metrics - right aligned to match Network RX 'G'
		fmt.Fprint(metricsPanel, formatMetric("HTTP Connections", fmt.Sprintf("%8s", formatNumber(int(getTotalHTTPConnections(nodesStats))))))
		fmt.Fprint(metricsPanel, formatMetric("Query Rate", fmt.Sprintf("%6s/s", formatNumber(int(queryRate)))))
		fmt.Fprint(metricsPanel, formatMetric("Index Rate", fmt.Sprintf("%6s/s", formatNumber(int(indexRate)))))

		// Snapshots
		fmt.Fprint(metricsPanel, formatMetric("Snapshots", fmt.Sprintf("%8s", formatNumber(clusterStats.Snapshots.Count))))

		if showRoles {
			updateRolesPanel(rolesPanel, nodesInfo)
		}
	}

	// Set up periodic updates
	go func() {
		for {
			app.QueueUpdateDraw(func() {
				update()
			})
			time.Sleep(5 * time.Second)
		}
	}()

	// Handle quit
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				app.Stop()
			case '2':
				showNodes = !showNodes
				updateGridLayout(grid, showRoles, showIndices, showMetrics)
			case '3':
				showRoles = !showRoles
				updateGridLayout(grid, showRoles, showIndices, showMetrics)
			case '4':
				showIndices = !showIndices
				updateGridLayout(grid, showRoles, showIndices, showMetrics)
			case '5':
				showMetrics = !showMetrics
				updateGridLayout(grid, showRoles, showIndices, showMetrics)
			case 'h':
				showHiddenIndices = !showHiddenIndices
				// Let the regular update cycle handle it
			}
		}
		return event
	})

	if err := app.SetRoot(grid, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func getTotalNetworkTX(stats NodesStats) int64 {
	var total int64
	for _, node := range stats.Nodes {
		total += node.Transport.TxSizeInBytes
	}
	return total
}

func getTotalNetworkRX(stats NodesStats) int64 {
	var total int64
	for _, node := range stats.Nodes {
		total += node.Transport.RxSizeInBytes
	}
	return total
}

func getMaxLengths(nodesInfo NodesInfo, indicesStats IndexStats) (int, int, int, int) {
	maxNodeNameLen := 0
	maxIndexNameLen := 0
	maxTransportLen := 0
	maxIngestedLen := 8 // Start with "Ingested" header length

	// Get max node name and transport address length
	for _, nodeInfo := range nodesInfo.Nodes {
		if len(nodeInfo.Name) > maxNodeNameLen {
			maxNodeNameLen = len(nodeInfo.Name)
		}
		if len(nodeInfo.TransportAddress) > maxTransportLen {
			maxTransportLen = len(nodeInfo.TransportAddress)
		}
	}

	// Get max index name length and calculate max ingested length
	for _, index := range indicesStats {
		if (showHiddenIndices || !strings.HasPrefix(index.Index, ".")) && index.DocsCount != "0" {
			if len(index.Index) > maxIndexNameLen {
				maxIndexNameLen = len(index.Index)
			}

			docs := 0
			fmt.Sscanf(index.DocsCount, "%d", &docs)
			if activity := indexActivities[index.Index]; activity != nil {
				if activity.InitialDocsCount < docs {
					docChange := docs - activity.InitialDocsCount
					ingestedStr := fmt.Sprintf("+%s", formatNumber(docChange))
					if len(ingestedStr) > maxIngestedLen {
						maxIngestedLen = len(ingestedStr)
					}
				}
			}
		}
	}

	// Add padding
	maxNodeNameLen += 2
	maxIndexNameLen += 1 // Changed from 2 to 1 for minimal padding
	maxTransportLen += 2
	maxIngestedLen += 1 // Minimal padding for ingested column

	return maxNodeNameLen, maxIndexNameLen, maxTransportLen, maxIngestedLen
}

func getNodesPanelHeader(maxNodeNameLen, maxTransportLen int) string {
	return fmt.Sprintf("[::b]%-*s [#444444]│[#00ffff] %-13s [#444444]│[#00ffff] %*s [#444444]│[#00ffff] %-7s [#444444]│[#00ffff] %-9s [#444444]│[#00ffff] %-16s [#444444]│[#00ffff] %-16s [#444444]│[#00ffff] %-16s [#444444]│[#00ffff] %-6s [#444444]│[#00ffff] %-25s[white]\n",
		maxNodeNameLen,
		"Node Name",
		"Roles",
		maxTransportLen,
		"Transport Address",
		"Version",
		"CPU",
		"Memory",
		"Heap",
		"Disk",
		"Uptime",
		"OS")
}

func getIndicesPanelHeader(maxIndexNameLen, maxIngestedLen int) string {
	return fmt.Sprintf("   [::b] %-*s [#444444]│[#00ffff] %13s [#444444]│[#00ffff] %5s [#444444]│[#00ffff] %6s [#444444]│[#00ffff] %8s [#444444]│[#00ffff] %-*s [#444444][#00ffff] %-8s[white]\n",
		maxIndexNameLen,
		"Index Name",
		"Documents",
		"Size",
		"Shards",
		"Replicas",
		maxIngestedLen,
		"Ingested",
		"Rate")
}

func isDataStream(name string, dataStreams DataStreamResponse) bool {
	for _, ds := range dataStreams.DataStreams {
		if ds.Name == name {
			return true
		}
	}
	return false
}

func getTotalSize(stats NodesStats) int64 {
	var total int64
	for _, node := range stats.Nodes {
		if len(node.FS.Data) > 0 {
			total += node.FS.Data[0].TotalInBytes - node.FS.Data[0].AvailableInBytes
		}
	}
	return total
}

func getTotalDiskSpace(stats NodesStats) int64 {
	var total int64
	for _, node := range stats.Nodes {
		if len(node.FS.Data) > 0 {
			total += node.FS.Data[0].TotalInBytes
		}
	}
	return total
}

func formatUptime(uptimeMillis int64) string {
	uptime := time.Duration(uptimeMillis) * time.Millisecond
	days := int(uptime.Hours() / 24)
	hours := int(uptime.Hours()) % 24
	minutes := int(uptime.Minutes()) % 60

	var result string
	if days > 0 {
		result = fmt.Sprintf("%d[#ff99cc]d[white]%d[#ff99cc]h[white]", days, hours)
	} else if hours > 0 {
		result = fmt.Sprintf("%d[#ff99cc]h[white]%d[#ff99cc]m[white]", hours, minutes)
	} else {
		result = fmt.Sprintf("%d[#ff99cc]m[white]", minutes)
	}

	// Calculate the actual display length by removing all color codes in one pass
	displayLen := len(strings.NewReplacer(
		"[#ff99cc]", "",
		"[white]", "",
	).Replace(result))

	// Add padding to make all uptime strings align (6 chars for display)
	padding := 6 - displayLen
	if padding > 0 {
		result = strings.TrimRight(result, " ") + strings.Repeat(" ", padding)
	}

	return result
}

func getTotalHTTPConnections(stats NodesStats) int64 {
	var total int64
	for _, node := range stats.Nodes {
		total += node.HTTP.CurrentOpen
	}
	return total
}

func updateRolesPanel(rolesPanel *tview.TextView, nodesInfo NodesInfo) {
	rolesPanel.Clear()
	fmt.Fprintf(rolesPanel, "[::b][#00ffff][[#ff5555]3[#00ffff]] Legend[::-]\n\n")

	// Add Node Roles title in cyan
	fmt.Fprintf(rolesPanel, "[::b][#00ffff]Node Roles[::-]\n")

	// Define role letters (same as in formatNodeRoles)
	roleMap := map[string]string{
		"master":                "M",
		"data":                  "D",
		"data_content":          "C",
		"data_hot":              "H",
		"data_warm":             "W",
		"data_cold":             "K",
		"data_frozen":           "F",
		"ingest":                "I",
		"ml":                    "L",
		"remote_cluster_client": "R",
		"transform":             "T",
		"voting_only":           "V",
		"coordinating_only":     "O",
	}

	// Create a map of active roles in the cluster
	activeRoles := make(map[string]bool)
	for _, node := range nodesInfo.Nodes {
		for _, role := range node.Roles {
			activeRoles[role] = true
		}
	}

	// Sort roles alphabetically by their letters
	var roles []string
	for role := range legendLabels {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool {
		return roleMap[roles[i]] < roleMap[roles[j]]
	})

	// Display each role with its color and description
	for _, role := range roles {
		color := roleColors[role]
		label := legendLabels[role]
		letter := roleMap[role]

		// If role is not active in cluster, use grey color for the label
		labelColor := "[white]"
		if !activeRoles[role] {
			labelColor = "[#444444]"
		}

		fmt.Fprintf(rolesPanel, "[%s]%s[white] %s%s\n", color, letter, labelColor, label)
	}

	// Add version status information
	fmt.Fprintf(rolesPanel, "\n[::b][#00ffff]Version Status[::-]\n")
	fmt.Fprintf(rolesPanel, "[green]⚫[white] Up to date\n")
	fmt.Fprintf(rolesPanel, "[yellow]⚫[white] Outdated\n")

	// Add index health status information
	fmt.Fprintf(rolesPanel, "\n[::b][#00ffff]Index Health[::-]\n")
	fmt.Fprintf(rolesPanel, "[green]⚫[white] All shards allocated\n")
	fmt.Fprintf(rolesPanel, "[#ffff00]⚫[white] Replica shards unallocated\n")
	fmt.Fprintf(rolesPanel, "[#ff5555]⚫[white] Primary shards unallocated\n")

	// Add index status indicators
	fmt.Fprintf(rolesPanel, "\n[::b][#00ffff]Index Status[::-]\n")
	fmt.Fprintf(rolesPanel, "[#5555ff]⚫[white] Active indexing\n")
	fmt.Fprintf(rolesPanel, "[#444444]⚪[white] No indexing\n")
	fmt.Fprintf(rolesPanel, "[#bd93f9]⚫[white] Data stream\n")
}

func formatResourceSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%4d B", bytes)
	}

	units := []string{"B", "K", "M", "G", "T", "P"}
	exp := 0
	val := float64(bytes)

	for val >= unit && exp < len(units)-1 {
		val /= unit
		exp++
	}

	return fmt.Sprintf("%3d%s", int(val), units[exp])
}

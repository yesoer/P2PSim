package fynegui

import (
	"distributed-sys-emulator/bus"
	"encoding/json"
	"math"
	"strconv"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"fyne.io/x/fyne/widget/diagramwidget"
)

type NetworkDiagram struct {
	fyne.Widget
	stateMu sync.Mutex

	// state data
	buttons []*widget.Button
	nodes   []node
	edges   []edge
}

var stopIcon = widget.NewIcon(theme.MediaStopIcon())
var playIcon = widget.NewIcon(theme.MediaPlayIcon())

type node struct {
	diagramwidget.DiagramNode
	isAwaiting bool
	isPaused   bool
}

type edge struct {
	*diagramwidget.BaseDiagramLink
	from int
	to   int
}

type point struct {
	x float64
	y float64
}

func NewNetworkDiagram(eb bus.EventBus, wcanvas fyne.Canvas) *NetworkDiagram {
	networkDiag := NetworkDiagram{}
	networkDiag.stateMu = sync.Mutex{}

	diag := diagramwidget.NewDiagramWidget("Network Diagram")

	eb.Bind(bus.SentToEvt, func(task bus.SendTask) {
		networkDiag.refreshNodeSent(task)
	})

	eb.Bind(bus.AwaitStartEvt, func(id bus.NodeId) {
		networkDiag.refreshNodeAwait(id)
	})

	eb.Bind(bus.AwaitEndEvt, func(sendTasks []bus.SendTask) {
		networkDiag.refreshTransmitted(sendTasks)
	})

	eb.Bind(bus.NetworkResizeEvt, func(resizeData bus.NetworkResize) {
		networkDiag.refreshButtons(eb, wcanvas, resizeData.Cnt)
		networkDiag.refreshNodes(diag, resizeData.Cnt)
		networkDiag.refreshConnections(diag, resizeData.Connections)
	})

	eb.Bind(bus.NetworkConnectionsEvt, func(newConnections bus.Connections) {
		networkDiag.refreshConnections(diag, newConnections)
	})

	eb.Bind(bus.ContinueNodesEvt, func() {
		networkDiag.refreshOnContinue(diag)
	})

	eb.Bind(bus.DebugNodesEvt, func() {
		networkDiag.setNodesRunning(true)
		networkDiag.Refresh()
	})

	eb.Bind(bus.StopNodesEvt, func() {
		networkDiag.setNodesRunning(false)
		networkDiag.Refresh()
	})

	eb.Bind(bus.StartNodesEvt, func() {
		networkDiag.setNodesRunning(true)
		networkDiag.setEdgesClean(diag)
		networkDiag.Refresh()
	})

	diag.Refresh()
	scroll := container.NewScroll(diag)
	networkDiag.Widget = scroll
	return &networkDiag
}

// when paused in debug mode call this function to refresh on continue
func (networkDiag *NetworkDiagram) refreshOnContinue(diag *diagramwidget.DiagramWidget) {
	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	networkDiag.setEdgesClean(diag)
	networkDiag.setNodesRunning(true)
	networkDiag.Refresh()
}

// set buttons, matched to corresponding nodes
func (networkDiag *NetworkDiagram) refreshButtons(eb bus.EventBus,
	wcanvas fyne.Canvas, nodeCnt int) {

	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	buttons := make([]*widget.Button, nodeCnt)
	nodeModals := make([]Modal, nodeCnt)

	onPress := func(i int) func() {
		return func() {
			p := nodeModals[i]
			p.Resize(fyne.NewSize(300, 300))
			p.Show()
		}
	}

	for i := range buttons {
		// init popup
		label := widget.NewLabel("Custom json data : ")
		errorLabel := widget.NewLabel("")
		errorLabel.Hide()

		jsonInput := widget.NewMultiLineEntry()
		jsonInput.PlaceHolder = `{"foo":"bar"}`
		jsonInput.Resize(fyne.NewSize(300, 300))
		jsonInput.OnChanged = func(s string) {

			// unmarshal string/check json format validity
			var data any
			err := json.Unmarshal([]byte(s), &data)
			if err != nil {
				errorLabel.SetText(err.Error())
				errorLabel.Show()
				return
			}

			changeData := bus.NodeData{TargetId: i, Data: data}
			evt := bus.Event{Type: bus.NodeDataChangeEvt, Data: changeData}
			eb.Publish(evt)
			errorLabel.Hide()
		}

		vstack := container.NewVBox(
			label,
			jsonInput,
			errorLabel)

		popup := NewModal(vstack, wcanvas)
		popup.Hide()
		nodeModals[i] = popup

		// init buttons
		nodeName := "Node " + strconv.Itoa(i)
		buttons[i] = widget.NewButton(nodeName, onPress(i))
		buttons[i].Resize(buttons[i].MinSize())
	}

	networkDiag.buttons = buttons
}

// refresh nodes depending on the new node count
func (networkDiag *NetworkDiagram) refreshNodes(
	diag *diagramwidget.DiagramWidget, nodeCnt int) {

	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	points := placePointsOnCircle(nodeCnt)

	// TODO : check if count changed
	for _, n := range networkDiag.nodes {
		diag.RemoveElement(n.GetDiagramElementID())
	}
	networkDiag.nodes = networkDiag.nodes[:0]

	for i, p := range points {
		// needs an inital size because its not set for the widget on the
		// initial pass
		diagWidth := diag.Size().Width
		if diagWidth == 0 {
			diagWidth = InitialWindowSize.Width
		}

		diagHeight := diag.Size().Height
		if diagHeight == 0 {
			diagHeight = InitialWindowSize.Height
		}

		ratiow := diagWidth / 100
		ratioh := diagHeight / 100

		// draw node
		x := ratiow * float32(p.x*.5)
		y := ratioh * float32(p.y*.5)

		nodeName := "Node" + strconv.Itoa(i)
		nodeButton := networkDiag.buttons[i]
		diagNode := diagramwidget.NewDiagramNode(diag, nodeButton, "Id:"+nodeName)
		diagNode.Move(fyne.Position{X: x, Y: y})
		newNode := node{diagNode, false, false}
		networkDiag.nodes = append(networkDiag.nodes, newNode)
	}
	networkDiag.Refresh()
}

// recreates all links depending on the given connections
func (networkDiag *NetworkDiagram) refreshConnections(
	diag *diagramwidget.DiagramWidget, connections bus.Connections) {

	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	// remove existing edges
	for _, e := range networkDiag.edges {
		diag.RemoveElement(e.GetDiagramElementID())
	}
	networkDiag.edges = networkDiag.edges[:0]

	// add edges based on new connections
	for _, c := range connections {
		networkDiag.createLink(c, diag)
	}
	networkDiag.Refresh()
}

// when a node has sent data (no matter whether it was transmitted successfully)
func (networkDiag *NetworkDiagram) refreshNodeSent(task bus.SendTask) {
	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	for _, e := range networkDiag.edges {
		if e.from == task.From && e.to == task.To {
			id := "Sent:" + strconv.Itoa(e.from) + ":" + strconv.Itoa(e.to)
			e.AddSourceAnchoredText(id, "sending")
		}
	}
	networkDiag.Refresh()
}

// when a node starts awaiting
func (networkDiag *NetworkDiagram) refreshNodeAwait(id bus.NodeId) {
	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	networkDiag.nodes[id].isAwaiting = true
	networkDiag.setInnerObj(id)
	networkDiag.Refresh()
}

// when successful transmissions have been received
func (networkDiag *NetworkDiagram) refreshTransmitted(sendTasks []bus.SendTask) {
	networkDiag.stateMu.Lock()
	defer networkDiag.stateMu.Unlock()

	for _, t := range sendTasks {
		// display synced data
		for _, e := range networkDiag.edges {
			if t.From == e.from && t.To == e.to {
				dataStr, _ := json.Marshal(t.Data)
				e.AddMidpointAnchoredText("sendTask", string(dataStr))
			}
		}

		// remove awaiting status on receiver and set both nodes to paused
		networkDiag.nodes[t.From].isPaused = true
		networkDiag.nodes[t.To].isPaused = true
		networkDiag.nodes[t.To].isAwaiting = false
		networkDiag.setInnerObj(bus.NodeId(t.To))
		networkDiag.setInnerObj(bus.NodeId(t.From))
	}
	networkDiag.Refresh()
}

// change the UI such that all nodes are viewed as running
func (networkDiag *NetworkDiagram) setNodesRunning(isRunning bool) {
	for nid := range networkDiag.nodes {
		networkDiag.nodes[nid].isPaused = !isRunning
		networkDiag.setInnerObj(bus.NodeId(nid))
	}
}

func (networkDiag *NetworkDiagram) setEdgesClean(diag *diagramwidget.DiagramWidget) {
	// reset edge source and midpoint decorations
	for _, e := range networkDiag.edges {
		// TODO : add functionality to remove decorations directly to fyne-x
		diag.RemoveElement(e.GetDiagramElementID())
		c := bus.Connection{From: e.from, To: e.to}
		networkDiag.createLink(c, diag)
	}
}

// creates a link between two nodes representing a conneciton
func (networkDiag *NetworkDiagram) createLink(c bus.Connection, diag *diagramwidget.DiagramWidget) {
	linkName := "Link" + strconv.Itoa(c.From) + ":" + strconv.Itoa(c.To)
	link := diagramwidget.NewDiagramLink(diag, linkName)
	link.SetSourcePad(networkDiag.nodes[c.From].GetEdgePad())
	link.SetTargetPad(networkDiag.nodes[c.To].GetEdgePad())
	link.AddTargetDecoration(diagramwidget.NewArrowhead())
	edge := edge{link, c.From, c.To}
	networkDiag.edges = append(networkDiag.edges, edge)
}

// sets the inner object of the specified node (including statuses etc.)
func (networkDiag *NetworkDiagram) setInnerObj(nodeId bus.NodeId) {
	innerObj := container.NewHBox()

	runningIcon := playIcon
	if networkDiag.nodes[nodeId].isPaused {
		runningIcon = stopIcon
	}
	innerObj.Add(runningIcon)

	if networkDiag.nodes[nodeId].isAwaiting {
		innerObj.Add(widget.NewLabel("Awaiting"))
	}

	btn := networkDiag.buttons[nodeId]
	innerObj.Add(btn)

	networkDiag.nodes[nodeId].SetInnerObject(innerObj)
}

// given n points, evenly distribute them on a cricle and return their positions
// assuming the circle is placed in a 100x100 square (50,50 is its center and its
// radius is 35. effectively accounting for some margin
func placePointsOnCircle(n int) []point {
	center := point{50, 50}
	radius := 35.

	var points []point
	angleIncrement := 2 * math.Pi / float64(n)
	for i := 0; i < n; i++ {
		angle := float64(i) * angleIncrement
		x := center.x + radius*math.Cos(angle)
		y := center.y + radius*math.Sin(angle)
		points = append(points, point{x, y})
	}

	return points
}

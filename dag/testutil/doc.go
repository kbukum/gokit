// Package testutil provides test helpers and mock implementations
// for the gokit/dag package.
//
// It includes mock nodes, graph builders, and a test component
// that implements testutil.TestComponent for integration with
// gokit's test infrastructure.
//
// Example:
//
//	func TestMyPipeline(t *testing.T) {
//	    graph := testutil.NewGraphBuilder().
//	        AddNode(testutil.NewMockNode("extract", "raw-data", nil)).
//	        AddNode(testutil.NewMockNode("transform", "processed", nil)).
//	        AddEdge("extract", "transform").
//	        Build()
//
//	    engine := &dag.Engine{}
//	    state := dag.NewState()
//	    result, err := engine.ExecuteBatch(context.Background(), graph, state)
//	    // ... assertions
//	}
package testutil

// Package testutil helps consumers test modules, ports and whole service compositions built with bootstrap.
//
// # Running modules in a test
//
// [NewApp] returns a quiet App for tests, and [Start] starts any App within a bounded setup budget and shuts it down when the test ends. [bootstrap.ValueModule] provides a port with a test double, [Capture] reads a port that the modules under test provide, and [RegisterListener] declares a loopback HTTP [Listener] whose URL the test can call. Capture is itself a module that needs the port, so a missing provider fails startup with the same [bootstrap.ModuleError] a service would see.
//
//	app := testutil.NewApp(t)
//	public := testutil.RegisterListener(t, app, "public")
//	runs := testutil.Capture(t, app, runs.ServicePort)
//	if err := app.Use(runs.Module(cfg), bootstrap.ValueModule("store", runs.StorePort, fakeStore)); err != nil {
//	    t.Fatal(err)
//	}
//	testutil.Start(t, app)
//	svc := runs()                      // the provided runs.Service
//	resp, err := http.Get(public.URL() + "/runs")
//
// To check that a service composition wires without starting it, compose the App and call [bootstrap.App.CheckModules].
//
// # Port contracts
//
// [ValidateRemoteShape] checks that a port's interface has a shape that can be served remotely. [Contract] runs one behavior suite against every implementation of a port, typically the module's in-process implementation and its client module's remote client, so a module can move between services without changing behavior.
package testutil

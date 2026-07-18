// Package triton implements an inference provider for NVIDIA Triton Inference Server.
//
// [Register] installs the provider through a [Factory], with model, version,
// and operation selection plus authorization applied per prediction
// so Triton deployments are reachable through the shared inference contract.
package triton

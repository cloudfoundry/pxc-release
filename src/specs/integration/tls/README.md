This test suite verifies TLS behavior of a PXC deployment.

The assumes are that the deployment was deployed with:

- spec.tls.required = true, rejecting any plaintext connections
- spec.tls.enforce_tls_v1_2 = true; rejecting attempts by clients to use old TLS protocol versions

This test will fail if either plaintext connections are allowed or older TLS versions are allowed in the environment.

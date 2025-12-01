# CSI Specification Compatibility Matrix
| Amazon EFS CSI Driver \ CSI Spec Version | v0.3.0| v1.1.0 | v1.2.0 |
|------------------------------------------|-------|--------|--------|
| master branch                            | no    | no     | yes    |
| v2.x.x                                   | no    | no     | yes    |
| v1.x.x                                   | no    | no     | yes    |
| v0.3.0                                   | no    | yes    | no     |
| v0.2.0                                   | no    | yes    | no     |
| v0.1.0                                   | yes   | no     | no     |

# Kubernetes Version Compability Matrix
| Amazon EFS CSI Driver \ Kubernetes Version | maturity | v1.11 | v1.12 | v1.13 | v1.14 | v1.15 | v1.16 | v1.17+ |
|--------------------------------------------|----------|-------|-------|-------|-------|-------|-------|--------|
| master branch                              | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v2.1.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v2.0.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.7.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.6.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.5.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.4.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.3.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.2.x                                     | GA       | no    | no    | no    | no    | no    | no    | yes    |
| v1.1.x                                     | GA       | no    | no    | no    | yes   | yes   | yes   | yes    |
| v1.0.x                                     | GA       | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.3.0                                     | beta     | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.2.0                                     | beta     | no    | no    | no    | yes   | yes   | yes   | yes    |
| v0.1.0                                     | alpha    | yes   | yes   | yes   | no    | no    | no    | no     |
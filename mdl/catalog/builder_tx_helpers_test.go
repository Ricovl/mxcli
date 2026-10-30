// SPDX-License-Identifier: Apache-2.0

package catalog

// wrapTx is the transaction a pass run on its own in a test writes through.
func wrapTx(tx CatalogTx) *builderTx {
	return newBuilderTx(tx, nil)
}

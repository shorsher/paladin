/*
 * Copyright © 2025 Kaleido, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */
package transaction

import (
	"context"

	"github.com/LFDT-Paladin/paladin/common/go/pkg/i18n"
	"github.com/LFDT-Paladin/paladin/common/go/pkg/log"
	"github.com/LFDT-Paladin/paladin/core/internal/msgs"
	"github.com/LFDT-Paladin/paladin/core/internal/sequencer/common"
)

func (t *coordinatorTransaction) hasDependenciesNotAssembled(ctx context.Context) bool {
	// PreAssemble.DependsOn can only be set when transactions have arrived in the same delegation request.
	// It is cleared when the dependent transaction is selected for assembly which means there is no way
	// that this can be cleared if a dependency has not yet been assembled.
	if t.dependencies.PreAssemble.DependsOn != nil {
		return true
	}
	return t.hasUnassembledChainedDependencies(ctx)
}

func (t *coordinatorTransaction) hasUnassembledChainedDependencies(ctx context.Context) bool {
	for _, depID := range t.dependencies.Chained.DependsOn {
		dep := t.grapher.TransactionByID(ctx, depID)
		if dep == nil {
			log.L(ctx).Error(i18n.NewError(ctx, msgs.MsgSequencerGrapherDependencyNotFound, depID))
			return true
		}
		state := dep.GetCurrentState()
		if state == State_Initial || state == State_PreAssembly_Blocked || state == State_Pooled {
			return true
		}
	}
	return false
}

func action_InitializeForNewAssembly(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	return txn.initializeForNewAssembly(ctx)
}

// Initializes (or re-initializes) the transaction as it arrives in the pool
func (t *coordinatorTransaction) initializeForNewAssembly(ctx context.Context) error {
	// Reset anything that might have been updated during an initial attempt to assembly, endorse and dispatch this TX. This is a no-op if this is the first
	// and only time we pool & assemble this transaction but if we're re-pooling for any reason we must clear the post-assembly and any post-assembly
	// dependencies from a previous version of the grapher.
	t.pt.PostAssembly = nil
	t.pt.PreparedPublicTransaction = nil
	t.pt.PreparedPrivateTransaction = nil
	t.chainedChildID = nil
	// Clear post-assembly dependencies. Chained dependencies are tracked separately and persist.
	t.dependencies.PostAssemble.DependsOn = nil
	t.dependencies.PostAssemble.PrereqOf = nil
	t.pendingPreDispatchRequest = nil
	t.grapher.ForgetMints(t.pt.ID)
	t.clearTimeoutSchedules()
	t.resetEndorsementRequests(ctx)
	t.engineIntegration.ResetTransactions(ctx, t.pt.ID)

	return nil
}

func action_ResetTransactionLocks(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	log.L(ctx).Debugf("resetting transaction locks for %s", txn.pt.ID.String())
	// Clear minted-state index immediately when resetting in-memory transaction state to avoid
	// later assembles binding to stale minters that have already been reset/reverted.
	txn.grapher.ForgetMints(txn.pt.ID)
	txn.engineIntegration.ResetTransactions(ctx, txn.pt.ID)
	return nil
}

func guard_HasUnassembledDependencies(ctx context.Context, txn *coordinatorTransaction) bool {
	return txn.hasDependenciesNotAssembled(ctx)
}

func action_NotifyDependentsOfReset(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	// We emit a DependencyResetEvent for chained and post assembly dependencies whenever we transition to
	// State_Pooled or State_PreAssembly_Blocked.
	// For the initial transition from State_Initial and the transition from State_Assembling to State_Pooled
	// the only dependents we expect are chained dependencies
	if err := txn.notifyDependentsOfReset(ctx); err != nil {
		return err
	}
	// Once dependents have been notified of reset, clear tracked dependencies so repeated reset
	// events while dispatched are no-ops and stale dependency links are dropped.
	// Clear post-assembly dependency links while preserving chained links across repool.
	txn.dependencies.PostAssemble.DependsOn = nil
	txn.dependencies.PostAssemble.PrereqOf = nil
	return nil
}

func (t *coordinatorTransaction) notifyDependentsOfReset(ctx context.Context) error {
	for _, dependentID := range append(t.dependencies.PostAssemble.PrereqOf, t.dependencies.Chained.PrereqOf...) {
		dependentTxn := t.grapher.TransactionByID(ctx, dependentID)
		if dependentTxn != nil {
			err := dependentTxn.HandleEvent(ctx, &DependencyResetEvent{
				BaseCoordinatorEvent: BaseCoordinatorEvent{
					TransactionID: dependentID,
				},
			})
			if err != nil {
				log.L(ctx).Errorf("error notifying dependent transaction %s of repool of transaction %s: %s", dependentID, t.pt.ID, err)
				return err
			}
		} else {
			// The only condition under which this branch should be reachable is if the dependent has failed on
			// assembly, which is a final state, and has been cleaned up from memory
			log.L(ctx).Warnf("notifyDependentsOfRepool: Dependent transaction %s not found in memory", dependentID)
		}
	}

	return nil
}

func action_RemovePreAssembleDependency(ctx context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.dependencies.PreAssemble.DependsOn = nil
	return nil
}

func action_AddPreAssemblePrereqOf(ctx context.Context, txn *coordinatorTransaction, event common.Event) error {
	e := event.(*NewPreAssembleDependencyEvent)
	txn.dependencies.PreAssemble.PrereqOf = &e.PrereqTransactionID
	return nil
}

func action_RemovePreAssemblePrereqOf(_ context.Context, txn *coordinatorTransaction, _ common.Event) error {
	txn.dependencies.PreAssemble.PrereqOf = nil
	return nil
}

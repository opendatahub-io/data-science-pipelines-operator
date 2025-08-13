package dspastatus

import (
	"fmt"

	dspav1 "github.com/opendatahub-io/data-science-pipelines-operator/api/v1"
	"github.com/opendatahub-io/data-science-pipelines-operator/controllers/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DSPAStatus interface {
	SetDatabaseReady()
	SetDatabaseNotReady(err error, reason string)

	SetObjStoreReady()
	SetObjStoreNotReady(err error, reason string)

	SetWebhookReady()
	SetWebhookNotReady(err error, reason string)
	SetWebhookNotApplicable()

	SetApiServerStatus(apiServerReady metav1.Condition)

	SetPersistenceAgentStatus(persistenceAgentReady metav1.Condition)

	SetScheduledWorkflowStatus(scheduledWorkflowReady metav1.Condition)

	SetWorkflowControllerStatus(workflowControllerReady metav1.Condition)
	SetWorkflowControllerNotReady(err error, reason string)
	SetWorkflowControllerReady()
	SetWorkflowControllerNotApplicable()

	SetMLMDProxyStatus(mlmdProxyReady metav1.Condition)

	SetDSPANotReady(err error, reason string)

	GetConditions() []metav1.Condition
}

func NewDSPAStatus(dspa *dspav1.DataSciencePipelinesApplication) DSPAStatus {
	databaseCondition := BuildUnknownCondition(config.DatabaseAvailable)
	objStoreCondition := BuildUnknownCondition(config.ObjectStoreAvailable)
	apiServerCondition := BuildUnknownCondition(config.APIServerReady)
	persistenceAgentCondition := BuildUnknownCondition(config.PersistenceAgentReady)
	scheduledWorkflowReadyCondition := BuildUnknownCondition(config.ScheduledWorkflowReady)
	workflowControllerReadyCondition := BuildUnknownCondition(config.WorkflowControllerReady)
	mlmdProxyReadyCondition := BuildUnknownCondition(config.MLMDProxyReady)
	webhookReadyCondition := BuildUnknownCondition(config.WebhookReady)

	return &dspaStatus{
		dspa:                    dspa,
		databaseAvailable:       &databaseCondition,
		objStoreAvailable:       &objStoreCondition,
		apiServerReady:          &apiServerCondition,
		persistenceAgentReady:   &persistenceAgentCondition,
		scheduledWorkflowReady:  &scheduledWorkflowReadyCondition,
		workflowControllerReady: &workflowControllerReadyCondition,
		mlmdProxyReady:          &mlmdProxyReadyCondition,
		webhookReady:            &webhookReadyCondition,
	}
}

type dspaStatus struct {
	dspa                    *dspav1.DataSciencePipelinesApplication
	databaseAvailable       *metav1.Condition
	objStoreAvailable       *metav1.Condition
	apiServerReady          *metav1.Condition
	persistenceAgentReady   *metav1.Condition
	scheduledWorkflowReady  *metav1.Condition
	workflowControllerReady *metav1.Condition
	mlmdProxyReady          *metav1.Condition
	dspaReady               *metav1.Condition
	webhookReady            *metav1.Condition
}

func (s *dspaStatus) SetDatabaseNotReady(err error, reason string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	condition := BuildFalseCondition(config.DatabaseAvailable, reason, message)
	s.databaseAvailable = &condition
}

func (s *dspaStatus) SetDatabaseReady() {
	condition := BuildTrueCondition(config.DatabaseAvailable, "Database connectivity successfully verified")
	s.databaseAvailable = &condition
}

func (s *dspaStatus) SetObjStoreReady() {
	condition := BuildTrueCondition(config.ObjectStoreAvailable, "Object Store connectivity successfully verified")
	s.objStoreAvailable = &condition
}

func (s *dspaStatus) SetObjStoreNotReady(err error, reason string) {
	message := ""
	if err != nil {
		message = err.Error()
	}

	condition := BuildFalseCondition(config.ObjectStoreAvailable, reason, message)
	s.objStoreAvailable = &condition
}

func (s *dspaStatus) SetWebhookNotReady(err error, reason string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	condition := BuildFalseCondition(config.WebhookReady, reason, message)
	s.webhookReady = &condition
}

func (s *dspaStatus) SetWebhookReady() {
	condition := BuildTrueCondition(config.WebhookReady, "Webhook deployment successfully verified")
	s.webhookReady = &condition
}

func (s *dspaStatus) SetWebhookNotApplicable() {
	condition := BuildFalseCondition(config.WebhookReady, "NotApplicable", "Webhook deployment not applicable for pipeline storage other than 'kubernetes'")
	s.webhookReady = &condition
}

func (s *dspaStatus) SetApiServerStatus(apiServerReady metav1.Condition) {
	s.apiServerReady = &apiServerReady
}

func (s *dspaStatus) SetPersistenceAgentStatus(persistenceAgentReady metav1.Condition) {
	s.persistenceAgentReady = &persistenceAgentReady
}

func (s *dspaStatus) SetScheduledWorkflowStatus(scheduledWorkflowReady metav1.Condition) {
	s.scheduledWorkflowReady = &scheduledWorkflowReady
}

func (s *dspaStatus) SetWorkflowControllerStatus(workflowControllerReady metav1.Condition) {
	s.workflowControllerReady = &workflowControllerReady
}

func (s *dspaStatus) SetWorkflowControllerNotReady(err error, reason string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	condition := BuildFalseCondition(config.WorkflowControllerReady, reason, message)
	s.workflowControllerReady = &condition
}

func (s *dspaStatus) SetWorkflowControllerReady() {
	condition := BuildTrueCondition(config.WorkflowControllerReady, "WorkflowController deployment successfully verified")
	s.workflowControllerReady = &condition
}

func (s *dspaStatus) SetWorkflowControllerNotApplicable() {
	condition := BuildFalseCondition(config.WorkflowControllerReady, "NotApplicable", "WorkflowController deployment disabled by Pipelines Operator")
	s.workflowControllerReady = &condition
}

func (s *dspaStatus) SetMLMDProxyStatus(mlmdProxyReady metav1.Condition) {
	s.mlmdProxyReady = &mlmdProxyReady
}

// SetDSPANotReady is an override option for reporting a custom
// overall DSP Ready state. This is the condition type that
// reports on the overall state of the DSPA. If this is never
// called, then the overall ready state is auto generated based
// on the conditions of the other components.
func (s *dspaStatus) SetDSPANotReady(err error, reason string) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	condition := BuildFalseCondition(config.CrReady, reason, message)
	s.dspaReady = &condition
}

func (s *dspaStatus) GetConditions() []metav1.Condition {
	componentConditions := []metav1.Condition{
		*s.getDatabaseAvailableCondition(),
		*s.getObjStoreAvailableCondition(),
		*s.getApiServerReadyCondition(),
		*s.getPersistenceAgentReadyCondition(),
		*s.getScheduledWorkflowReadyCondition(),
		*s.getWorkflowControllerReadyCondition(),
		*s.getMLMDProxyReadyCondition(),
		*s.getWebhookReadyCondition(),
	}

	allReady := true
	failureMessages := ""
	for _, c := range componentConditions {
		if (c.Status == metav1.ConditionFalse || c.Status == metav1.ConditionUnknown) && c.Reason != "NotApplicable" {
			allReady = false
			failureMessages += fmt.Sprintf("%s \n", c.Message)
		}
	}

	// Allow for dspa ready status to be overridden
	// otherwise we auto generate the overall ready status
	// based off of the other components
	crReady := s.dspaReady

	if s.dspaReady == nil {
		if allReady {
			crReady = &metav1.Condition{
				Type:               config.CrReady,
				Status:             metav1.ConditionTrue,
				Reason:             config.MinimumReplicasAvailable,
				Message:            "All components are ready.",
				LastTransitionTime: metav1.Now(),
			}
		} else {
			crReady = &metav1.Condition{
				Type:               config.CrReady,
				Status:             metav1.ConditionFalse,
				Reason:             config.MinimumReplicasAvailable,
				Message:            failureMessages,
				LastTransitionTime: metav1.Now(),
			}
		}
	}

	conditions := []metav1.Condition{
		*s.databaseAvailable,
		*s.objStoreAvailable,
		*s.apiServerReady,
		*s.persistenceAgentReady,
		*s.scheduledWorkflowReady,
		*s.workflowControllerReady,
		*s.mlmdProxyReady,
		*s.webhookReady,
		*crReady,
	}

	for i, condition := range s.dspa.Status.Conditions {
		if condition.Status == conditions[i].Status {
			conditions[i].LastTransitionTime = condition.LastTransitionTime
		}
		condition.ObservedGeneration = s.dspa.Generation
	}

	return conditions
}

func (s *dspaStatus) getDatabaseAvailableCondition() *metav1.Condition {
	return s.databaseAvailable
}

func (s *dspaStatus) getObjStoreAvailableCondition() *metav1.Condition {
	return s.objStoreAvailable
}

func (s *dspaStatus) getApiServerReadyCondition() *metav1.Condition {
	return s.apiServerReady
}

func (s *dspaStatus) getPersistenceAgentReadyCondition() *metav1.Condition {
	return s.persistenceAgentReady
}

func (s *dspaStatus) getScheduledWorkflowReadyCondition() *metav1.Condition {
	return s.scheduledWorkflowReady
}

func (s *dspaStatus) getWorkflowControllerReadyCondition() *metav1.Condition {
	return s.workflowControllerReady
}

func (s *dspaStatus) getMLMDProxyReadyCondition() *metav1.Condition {
	return s.mlmdProxyReady
}

func (s *dspaStatus) getWebhookReadyCondition() *metav1.Condition {
	return s.webhookReady
}

func BuildTrueCondition(conditionType string, message string) metav1.Condition {
	condition := metav1.Condition{}
	condition.Type = conditionType
	condition.Status = metav1.ConditionTrue
	condition.Reason = conditionType
	condition.Message = message
	condition.LastTransitionTime = metav1.Now()

	return condition
}

func BuildFalseCondition(conditionType string, reason string, message string) metav1.Condition {
	condition := metav1.Condition{}
	condition.Type = conditionType
	condition.Status = metav1.ConditionFalse
	condition.Reason = reason
	condition.Message = message
	condition.LastTransitionTime = metav1.Now()

	return condition
}

func BuildUnknownCondition(conditionType string) metav1.Condition {
	condition := metav1.Condition{}
	condition.Type = conditionType
	condition.Status = metav1.ConditionUnknown
	condition.Reason = "Unknown"
	condition.LastTransitionTime = metav1.Now()

	return condition
}

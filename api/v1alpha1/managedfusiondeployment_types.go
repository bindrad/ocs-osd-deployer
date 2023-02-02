/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type SMTPSpec struct {
	Endpoint           string   `json:"endpoint,omitempty"`
	Username           string   `json:"username,omitempty"`
	FromAddress        string   `json:"fromAddress,omitempty"`
	NotificationEmails []string `json:"notificationEmails,omitempty"`
}

type PagerSpec struct {
	SOPEndpoint string `json:"sopEndpoint,omitempty"`
}

// ManagedFusionDeploymentSpec defines the desired state of ManagedFusionDeployment
type ManagedFusionDeploymentSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ManagedFusionDeployment. Edit managedfusiondeployment_types.go to remove/update
	SMTP  SMTPSpec  `json:"smtp,omitempty"`
	Pager PagerSpec `json:"pager,omitempty"`
}

// ManagedFusionDeploymentStatus defines the observed state of ManagedFusionDeployment
type ManagedFusionDeploymentStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ManagedFusionDeployment is the Schema for the managedfusiondeployments API
type ManagedFusionDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedFusionDeploymentSpec   `json:"spec,omitempty"`
	Status ManagedFusionDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagedFusionDeploymentList contains a list of ManagedFusionDeployment
type ManagedFusionDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedFusionDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedFusionDeployment{}, &ManagedFusionDeploymentList{})
}

package cloudflarerecord

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	cloudflareoperatorv1alpha1 "github.com/ghetzel/cloudflare-operator/pkg/apis/cloudflareoperator/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_cloudflarerecord")

// Add creates a new CloudflareRecord Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileCloudflareRecord{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("cloudflarerecord-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource CloudflareRecord
	err = c.Watch(&source.Kind{Type: &cloudflareoperatorv1alpha1.CloudflareRecord{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileCloudflareRecord implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileCloudflareRecord{}

// ReconcileCloudflareRecord reconciles a CloudflareRecord object
type ReconcileCloudflareRecord struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a CloudflareRecord object and makes changes based on the state read
// and what is in the CloudflareRecord.Spec
func (r *ReconcileCloudflareRecord) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling CloudflareRecord")

	// Fetch the CloudflareRecord instance
	var instance = new(cloudflareoperatorv1alpha1.CloudflareRecord)
	err := r.client.Get(context.Background(), request.NamespacedName, instance)

	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	var cfaccount = os.Getenv(`CLOUDFLARE_ACCOUNT`)
	var apikey = os.Getenv(`CLOUDFLARE_APIKEY`)

	if cfaccount == `` {
		return reconcile.Result{}, fmt.Errorf("Must provide CLOUDFLARE_ACCOUNT environment variable")
	}

	if apikey == `` {
		return reconcile.Result{}, fmt.Errorf("Must provide CLOUDFLARE_APIKEY environment variable")
	}

	// setup CloudFlare API
	cf, err := cloudflare.New(apikey, cfaccount)

	if err != nil {
		return reconcile.Result{}, err
	}

	// get the zone referred to by the instance we're reconciling
	zoneID, err := cf.ZoneIDByName(instance.Spec.Zone)

	if err != nil {
		return reconcile.Result{}, err
	}

	// try to find an existing DNS record for this Type+Name
	if matches, err := cf.DNSRecords(zoneID, cloudflare.DNSRecord{}); err == nil {
		var update bool
		var rr = cloudflare.DNSRecord{
			Type:    string(instance.Spec.Type),
			Name:    instance.Spec.Name,
			Content: instance.Spec.Content,
			TTL:     instance.Spec.TTL,
			Proxied: instance.Spec.Proxied,
			ZoneID:  zoneID,
		}

		if rr.TTL == 0 {
			rr.TTL = 1
		}

		reqLogger.Info(fmt.Sprintf("Checking Cloudflare DNS record [%s] %s", rr.Type, rr.Name))

		for _, match := range matches {
			if match.Type == rr.Type {
				var fullname = instance.Spec.Name

				if !strings.HasSuffix(fullname, `.`+instance.Spec.Zone) {
					fullname += `.` + instance.Spec.Zone
				}

				if match.Name == fullname {
					rr.ID = matches[0].ID
					rr.Name = fullname
					update = true

					reqLogger.Info(fmt.Sprintf("Matched record ID %v, checking for update", rr.ID))
					break
				}
			}
		}

		var err error

		if update {
			// only perform the update call if any of the spec data differs from what's already in CloudFlare
			if (matches[0].Type != rr.Type) ||
				(matches[0].Name != rr.Name) ||
				(matches[0].Content != rr.Content) ||
				(matches[0].Proxied != rr.Proxied) ||
				(matches[0].TTL != rr.TTL) {
				reqLogger.Info("Updating Cloudflare DNS record", "Name", rr.Name, "Type", rr.Type, "Content", rr.Content, "Proxied", rr.Proxied)

				rr.Name = `` // let the CloudFlare library figure this out

				err = cf.UpdateDNSRecord(zoneID, rr.ID, rr)
			} else {
				reqLogger.Info("No changes necessary", "Name", rr.Name, "Type", rr.Type, "Content", rr.Content, "Proxied", rr.Proxied)
			}
		} else {
			reqLogger.Info("Creating Cloudflare DNS record", "Name", rr.Name, "Type", rr.Type, "Content", rr.Content, "Proxied", rr.Proxied)
			_, err = cf.CreateDNSRecord(zoneID, rr)
		}

		reqLogger.Info("Records synced", "Name", rr.Name, "Type", rr.Type, "Content", rr.Content, "Proxied", rr.Proxied)

		return reconcile.Result{}, err
	} else {
		return reconcile.Result{}, err
	}
}

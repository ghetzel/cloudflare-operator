package cloudflarerecord

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/cloudflare/cloudflare-go"
	cloudflareoperatorv1alpha1 "github.com/ghetzel/cloudflare-operator/pkg/apis/cloudflareoperator/v1alpha1"
	slog "github.com/ghetzel/go-stockutil/log"
	"github.com/ghetzel/go-stockutil/sliceutil"
	"github.com/ghetzel/go-stockutil/stringutil"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const cloudflareRecordFinalizer = `finalizer.cloudflarerecord.cloudflare-operator.gary.cool`

var log = logf.Log.WithName("controller_cloudflarerecord")

type targetAddressFunc func(ctx context.Context, key client.ObjectKey) (string, error)

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
	cf     *cloudflare.API
}

// Reconcile reads that state of the cluster for a CloudflareRecord object and makes changes based on the state read
// and what is in the CloudflareRecord.Spec
func (r *ReconcileCloudflareRecord) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	var ctx = context.Background()

	slog.SetLevel(slog.DEBUG)

	// Fetch the CloudflareRecord instance
	var instance = new(cloudflareoperatorv1alpha1.CloudflareRecord)
	err := r.client.Get(ctx, request.NamespacedName, instance)

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

	if err := r.setupCloudflareClient(); err != nil {
		return reconcile.Result{}, err
	}

	var targetAddress string
	var objspec string
	var tafunc targetAddressFunc

	if instance.Spec.Service != `` {
		objspec = instance.Spec.Service
		tafunc = r.targetAddressFromService
	} else if instance.Spec.Ingress != `` {
		objspec = instance.Spec.Ingress
		tafunc = r.targetAddressFromIngress
	}

	if tafunc != nil {
		var ns, name = stringutil.SplitPairTrailing(objspec, `/`)

		if ns == `` {
			ns = instance.Namespace
		}

		if ta, err := tafunc(ctx, client.ObjectKey{
			Namespace: ns,
			Name:      name,
		}); err == nil {
			targetAddress = ta
		} else {
			return reconcile.Result{}, err
		}

	}

	if targetAddress == `` {
		targetAddress = instance.Spec.Content
	}

	if targetAddress == `` {
		slog.Errorf("Service target %q: but no usable address found", objspec)
		return reconcile.Result{}, nil
	}

	// get the zone referred to by the instance we're reconciling
	zoneID, err := r.cf.ZoneIDByName(instance.Spec.Zone)

	if err != nil {
		return reconcile.Result{}, err
	}

	if rr, hasChanged, err := r.firstRecordByInstance(zoneID, targetAddress, instance); err == nil {
		if instance.GetDeletionTimestamp() == nil {
			slog.Debugf("Checking Cloudflare DNS record: %v %s", rr.Type, rr.Name)

			if rr.ID != `` {
				// only perform the update call if any of the spec data differs from what's already in CloudFlare
				if hasChanged {
					if rr.Proxied {
						slog.Noticef("Updating Cloudflare DNS record: %v %s -> {CloudFlare} -> %v", rr.Type, rr.Name, rr.Content)
					} else {
						slog.Noticef("Updating Cloudflare DNS record: %v %s -> %v", rr.Type, rr.Name, rr.Content)
					}

					rr.Name = `` // let the CloudFlare library figure this out

					err = r.cf.UpdateDNSRecord(zoneID, rr.ID, rr)
				}
			} else {
				if rr.Proxied {
					slog.Noticef("Creating Cloudflare DNS record: %v %s -> {CloudFlare} -> %v", rr.Type, rr.Name, rr.Content)
				} else {
					slog.Noticef("Creating Cloudflare DNS record: %v %s -> %v", rr.Type, rr.Name, rr.Content)
				}

				_, err = r.cf.CreateDNSRecord(zoneID, rr)
			}
		} else if rr.ID != `` {
			slog.Warningf("Removing Cloudflare DNS record %v %s", rr.Type, rr.Name)

			if sliceutil.ContainsString(instance.GetFinalizers(), cloudflareRecordFinalizer) {
				if err := r.finalize(zoneID, rr.ID); err != nil {
					return reconcile.Result{}, err
				}

				// Remove finalizer. Once all finalizers have been removed, the object will be deleted.
				controllerutil.RemoveFinalizer(instance, cloudflareRecordFinalizer)

				if err := r.client.Update(context.Background(), instance); err != nil {
					return reconcile.Result{}, err
				}
			}
		}

		// Add finalizer for this CR
		if !sliceutil.ContainsString(instance.GetFinalizers(), cloudflareRecordFinalizer) {
			if err := r.addFinalizer(instance); err != nil {
				return reconcile.Result{}, err
			}
		}

		return reconcile.Result{}, err
	} else {
		return reconcile.Result{}, err
	}
}

func (r *ReconcileCloudflareRecord) targetAddressFromService(ctx context.Context, key client.ObjectKey) (string, error) {
	var service = new(corev1.Service)

	if err := r.client.Get(ctx, key, service); err == nil {
		switch service.Spec.Type {
		case corev1.ServiceTypeLoadBalancer:
			for _, ingress := range service.Status.LoadBalancer.Ingress {
				if ingress.Hostname != `` {
					return ingress.Hostname, nil
				} else if ingress.IP != `` {
					return ingress.IP, nil
				}
			}
		case corev1.ServiceTypeExternalName:
			if en := service.Spec.ExternalName; en != `` {
				return en, nil
			}
		default:
			if cip := service.Spec.ClusterIP; cip != `` {
				return cip, nil
			}
		}
	} else {
		return ``, err
	}

	return ``, fmt.Errorf("no suitable service address found")
}

func (r *ReconcileCloudflareRecord) targetAddressFromIngress(ctx context.Context, key client.ObjectKey) (string, error) {
	var ingress = new(extv1.Ingress)

	if err := r.client.Get(ctx, key, ingress); err == nil {
		for _, iglb := range ingress.Status.LoadBalancer.Ingress {
			if iglb.Hostname != `` {
				return iglb.Hostname, nil
			} else if iglb.IP != `` {
				return iglb.IP, nil
			}
		}
	} else {
		return ``, err
	}

	return ``, fmt.Errorf("no suitable service address found")
}

func (r *ReconcileCloudflareRecord) setupCloudflareClient() error {
	if r.cf == nil {
		var cfaccount = os.Getenv(`CLOUDFLARE_ACCOUNT`)
		var apikey = os.Getenv(`CLOUDFLARE_APIKEY`)

		if cfaccount == `` {
			return fmt.Errorf("Must provide CLOUDFLARE_ACCOUNT environment variable")
		}

		if apikey == `` {
			return fmt.Errorf("Must provide CLOUDFLARE_APIKEY environment variable")
		}

		// setup CloudFlare API
		if cf, err := cloudflare.New(apikey, cfaccount); err == nil {
			r.cf = cf
		} else {
			return err
		}
	}

	if r.cf == nil {
		return fmt.Errorf("Failed to setup CloudFlare API")
	}

	return nil
}

func (r *ReconcileCloudflareRecord) addFinalizer(record *cloudflareoperatorv1alpha1.CloudflareRecord) error {
	slog.Debugf("Adding Finalizer for CloudflareRecord")
	controllerutil.AddFinalizer(record, cloudflareRecordFinalizer)

	// Update CR
	err := r.client.Update(context.Background(), record)
	if err != nil {
		slog.Errorf("Failed to update CloudflareRecord with finalizer: %v", err)
		return err
	}
	return nil
}

func (r *ReconcileCloudflareRecord) firstRecordByInstance(zoneID string, targetAddress string, record *cloudflareoperatorv1alpha1.CloudflareRecord) (cloudflare.DNSRecord, bool, error) {
	var hasChanged bool

	// try to find an existing DNS record for this Type+Name
	if matches, err := r.cf.DNSRecords(zoneID, cloudflare.DNSRecord{}); err == nil {
		var rr = cloudflare.DNSRecord{
			Type:    string(record.Spec.Type),
			Name:    record.Spec.Name,
			Content: targetAddress,
			TTL:     record.Spec.TTL,
			Proxied: record.Spec.Proxied,
			ZoneID:  zoneID,
		}

		for _, match := range matches {
			if match.Type == rr.Type {
				var fullname = record.Spec.Name

				if !strings.HasSuffix(fullname, `.`+record.Spec.Zone) {
					fullname += `.` + record.Spec.Zone
				}

				if match.Name == fullname {
					rr.ID = match.ID
					rr.Name = fullname

					if (match.Type != rr.Type) ||
						(match.Name != rr.Name) ||
						(match.Content != rr.Content) ||
						(match.Proxied != rr.Proxied) ||
						(match.TTL != rr.TTL) {
						hasChanged = true
					}

					break
				}
			}
		}

		return rr, hasChanged, nil
	} else {
		return cloudflare.DNSRecord{}, false, err
	}
}

func (r *ReconcileCloudflareRecord) finalize(zoneID string, recordID string) error {
	if r.cf == nil {
		return fmt.Errorf("CloudFlare API not available")
	} else {
		r.cf.DeleteDNSRecord(zoneID, recordID)
		return nil
	}
}

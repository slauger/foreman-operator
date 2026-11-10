package controller

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	foremanv1alpha1 "github.com/slauger/foreman-operator/api/v1alpha1"
)

// Keys of the Foreman secrets Secret.
const (
	keyEncryptionKey       = "encryption-key"
	keySecretKeyBase       = "secret-key-base"
	keyAdminPassword       = "admin-password"
	keyOAuthConsumerKey    = "oauth-consumer-key"
	keyOAuthConsumerSecret = "oauth-consumer-secret" //nolint:gosec // a Secret key name, not a credential
)

// Components of a Foreman.
const (
	componentWeb               = "web"
	componentDynflowOrchestr   = "dynflow-orchestrator"
	componentDynflowWorker     = "dynflow-worker"
	componentDynflowHostsQueue = "dynflow-hosts-queue"
	componentMigration         = "migration"

	foremanAppName = "foreman"
	foremanWebPort = 3000

	foremanTLSMountPath = "/etc/foreman/tls"
)

// Valkey databases used by Foreman. Pulp will use database 8 later.
const (
	valkeyDBCache   = 4
	valkeyDBDynflow = 6
)

func foremanSecretsName(f *foremanv1alpha1.Foreman) string {
	if f.Spec.SecretsRef != "" {
		return f.Spec.SecretsRef
	}
	return f.Name + "-secrets"
}

func foremanSettingsName(f *foremanv1alpha1.Foreman) string { return f.Name + "-settings" }
func foremanDynflowName(f *foremanv1alpha1.Foreman) string  { return f.Name + "-dynflow" }

// foremanClientCertName is the client certificate Foreman presents to Smart Proxies.
func foremanClientCertName(f *foremanv1alpha1.Foreman) string {
	return tlsSecretName(f.Spec.TLS, f.Name+"-client")
}

// foremanWebServiceName is the in-cluster Service of the web workload.
func foremanWebServiceName(f *foremanv1alpha1.Foreman) string { return f.Name + "-web" }

func foremanComponentName(f *foremanv1alpha1.Foreman, component string) string {
	return f.Name + "-" + component
}

func foremanDBSecretName(f *foremanv1alpha1.Foreman) string {
	if f.Spec.Database.Cluster.CredentialsSecretRef != "" {
		return f.Spec.Database.Cluster.CredentialsSecretRef
	}
	return f.Spec.Database.Cluster.Name + "-app"
}

func foremanPublicURL(f *foremanv1alpha1.Foreman) string { return "https://" + f.Spec.FQDN }

func foremanImage(f *foremanv1alpha1.Foreman, images Images) string {
	return imageRef(f.Spec.Image, images.Foreman, images.Tag)
}

// migrationHash identifies a database schema state: the image and the enabled plugins.
func migrationHash(f *foremanv1alpha1.Foreman, images Images) string {
	plugins := slices.Clone(f.Spec.Plugins)
	slices.Sort(plugins)
	return checksum(append([]string{foremanImage(f, images)}, plugins...)...)[:16]
}

func migrationJobName(f *foremanv1alpha1.Foreman, hash string) string {
	return f.Name + "-migrate-" + hash[:8]
}

func valkeyURL(f *foremanv1alpha1.Foreman, db int) string {
	port := f.Spec.Valkey.Port
	if port == 0 {
		port = 6379
	}
	return fmt.Sprintf("redis://%s:%d/%d", f.Spec.Valkey.Host, port, db)
}

// foremanSettings renders /etc/foreman/settings.yaml. It contains the OAuth secret and is
// therefore stored in a Secret.
func foremanSettings(f *foremanv1alpha1.Foreman, oauthKey, oauthSecret string) string {
	hosts := make([]string, 0, len(f.Spec.Aliases)+3)
	hosts = append(hosts, f.Spec.FQDN)
	hosts = append(hosts, f.Spec.Aliases...)
	hosts = append(hosts,
		foremanWebServiceName(f),
		foremanWebServiceName(f)+"."+f.Namespace+".svc",
	)

	domain := ""
	if _, rest, ok := strings.Cut(f.Spec.FQDN, "."); ok {
		domain = rest
	}

	cache := rubyMap{}
	cache.set("type", "redis")
	cache.set("urls", []string{valkeyURL(f, valkeyDBCache)})
	cacheOptions := rubyMap{}
	cacheOptions.set("compress", true)
	cacheOptions.set("namespace", "foreman")
	cache.set("options", cacheOptions)

	s := rubyMap{}
	s.set("unattended", true)
	s.set("domain", domain)
	s.set("fqdn", f.Spec.FQDN)
	s.set("foreman_url", foremanPublicURL(f))
	s.set("hosts", hosts)
	// The Frontend and the operator reach Foreman from the pod network.
	s.set("trusted_proxies", []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	s.set("require_ssl", true)
	s.set("rails_cache_store", cache)
	s.set("ssl_client_dn_env", "HTTP_SSL_CLIENT_S_DN")
	s.set("ssl_client_verify_env", "HTTP_SSL_CLIENT_VERIFY")
	s.set("ssl_client_cert_env", "HTTP_SSL_CLIENT_CERT")
	s.set("ssl_certificate", foremanTLSMountPath+"/tls.crt")
	s.set("ssl_priv_key", foremanTLSMountPath+"/tls.key")
	s.set("ssl_ca_file", foremanTLSMountPath+"/ca.crt")
	s.set("oauth_active", true)
	s.set("oauth_map_users", false)
	s.set("oauth_consumer_key", oauthKey)
	s.set("oauth_consumer_secret", oauthSecret)
	return s.String()
}

// dynflowConfigs renders the Sidekiq configurations of the Dynflow workloads.
func dynflowConfigs() map[string]string {
	cfg := func(concurrency int, queues ...string) string {
		m := rubyMap{}
		m.set("concurrency", concurrency)
		m.set("queues", queues)
		return m.String()
	}
	return map[string]string{
		"orchestrator.yml":       cfg(1, "dynflow_orchestrator"),
		"worker.yml":             cfg(5, "default", "remote_execution"),
		"worker-hosts-queue.yml": cfg(5, "hosts_queue"),
	}
}

// pumaWorkers derives the Puma worker count from the memory limit, about 1.5 GiB each.
func pumaWorkers(res corev1.ResourceRequirements) int64 {
	limit, ok := res.Limits[corev1.ResourceMemory]
	if !ok {
		return 2
	}
	workers := limit.Value() / (1536 * 1024 * 1024)
	return max(1, min(workers, 16))
}

func defaultResources(res corev1.ResourceRequirements, cpu, memRequest, memLimit string) corev1.ResourceRequirements {
	if len(res.Requests) > 0 || len(res.Limits) > 0 {
		return res
	}
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpu),
			corev1.ResourceMemory: resource.MustParse(memRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(memLimit),
		},
	}
}

func secretEnv(name, secret, key string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				Name: secret,
				Key:  key,
			},
		},
	}
}

// foremanEnv is shared by all Foreman workloads and the migration Job.
func foremanEnv(f *foremanv1alpha1.Foreman, dbPool int) []corev1.EnvVar {
	secrets := foremanSecretsName(f)
	return []corev1.EnvVar{
		secretEnv("DB_URI", foremanDBSecretName(f), "uri"),
		{Name: "DATABASE_URL", Value: "$(DB_URI)?pool=" + strconv.Itoa(dbPool) + "&sslmode=require"},
		secretEnv("ENCRYPTION_KEY", secrets, keyEncryptionKey),
		secretEnv("SECRET_KEY_BASE", secrets, keySecretKeyBase),
		{Name: "DYNFLOW_REDIS_URL", Value: valkeyURL(f, valkeyDBDynflow)},
		{Name: "REDIS_PROVIDER", Value: "DYNFLOW_REDIS_URL"},
	}
}

func foremanVolumes(f *foremanv1alpha1.Foreman) []corev1.Volume {
	return []corev1.Volume{
		{Name: "settings", Secret: &corev1.SecretVolumeSource{SecretName: foremanSettingsName(f)}},
		{Name: "dynflow", ConfigMap: &corev1.ConfigMapVolumeSource{
			Name: foremanDynflowName(f),
		}},
		{Name: "tls", Secret: &corev1.SecretVolumeSource{SecretName: foremanClientCertName(f)}},
		{Name: "tmp", EmptyDir: &corev1.EmptyDirVolumeSource{}},
		{Name: "rails-tmp", EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}
}

func foremanVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: "settings", MountPath: "/etc/foreman/settings.yaml", SubPath: "settings.yaml", ReadOnly: true},
		{Name: "dynflow", MountPath: "/etc/foreman/dynflow", ReadOnly: true},
		{Name: "tls", MountPath: foremanTLSMountPath, ReadOnly: true},
		{Name: "tmp", MountPath: "/tmp"},
		{Name: "rails-tmp", MountPath: "/usr/share/foreman/tmp"},
	}
}

// foremanPodSpec builds the pod spec shared by the web and Dynflow workloads.
func foremanPodSpec(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, ctr corev1.Container) corev1.PodSpec {
	ctr.Image = foremanImage(f, images)
	ctr.ImagePullPolicy = pullPolicy(f.Spec.Image)
	ctr.SecurityContext = restrictedContainerSecurityContext()
	ctr.VolumeMounts = foremanVolumeMounts()
	return corev1.PodSpec{
		SecurityContext:               podSecurityContext(caps.OpenShift),
		TerminationGracePeriodSeconds: new(int64(60)),
		Containers:                    []corev1.Container{ctr},
		Volumes:                       foremanVolumes(f),
	}
}

func foremanDeployment(f *foremanv1alpha1.Foreman, component string, replicas int32, podSpec corev1.PodSpec, configChecksum string) *appsv1.Deployment {
	labels := commonLabels(foremanAppName, f.Name, component)
	d := &appsv1.Deployment{}
	d.Name = foremanComponentName(f, component)
	d.Namespace = f.Namespace
	d.Labels = labels
	d.Spec = appsv1.DeploymentSpec{
		Replicas: new(replicas),
		Selector: &metav1.LabelSelector{MatchLabels: selectorLabels(foremanAppName, f.Name, component)},
		Template: corev1.PodTemplateSpec{
			Spec: podSpec,
		},
	}
	d.Spec.Template.Labels = labels
	d.Spec.Template.Annotations = map[string]string{checksumAnnotation: configChecksum}
	return d
}

func foremanWebDeployment(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, replicas int32, configChecksum string) *appsv1.Deployment {
	res := defaultResources(f.Spec.Web.Resources, "200m", "1Gi", "3Gi")
	workers := pumaWorkers(res)
	probe := &corev1.Probe{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/api/v2/ping",
			Port: intstr.FromInt32(foremanWebPort),
			HTTPHeaders: []corev1.HTTPHeader{
				{Name: "Host", Value: f.Spec.FQDN},
				{Name: "X-Forwarded-Proto", Value: "https"},
			},
		},
		PeriodSeconds:    10,
		TimeoutSeconds:   5,
		FailureThreshold: 3,
	}
	startup := probe.DeepCopy()
	startup.FailureThreshold = 60

	ctr := corev1.Container{
		Name:  "foreman",
		Ports: []corev1.ContainerPort{{Name: "http", ContainerPort: foremanWebPort}},
		Env: append(foremanEnv(f, 5*int(workers)+4),
			corev1.EnvVar{Name: "FOREMAN_PUMA_WORKERS", Value: strconv.FormatInt(workers, 10)},
			corev1.EnvVar{Name: "FOREMAN_PUMA_THREADS_MIN", Value: "5"},
			corev1.EnvVar{Name: "FOREMAN_PUMA_THREADS_MAX", Value: "5"},
		),
		StartupProbe:   startup,
		ReadinessProbe: probe,
		LivenessProbe: &corev1.Probe{
			TCPSocket:        &corev1.TCPSocketAction{Port: intstr.FromInt32(foremanWebPort)},
			PeriodSeconds:    20,
			FailureThreshold: 6,
		},
		Resources: res,
	}
	return foremanDeployment(f, componentWeb, replicas, foremanPodSpec(f, images, caps, ctr), configChecksum)
}

func dynflowDeployment(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, component, config string, replicas int32, configChecksum string) *appsv1.Deployment {
	res := defaultResources(f.Spec.Dynflow.Resources, "100m", "512Mi", "1536Mi")
	ctr := corev1.Container{
		Name:      "sidekiq",
		Command:   []string{"sidekiq", "-e", "production", "-r", "./extras/dynflow-sidekiq.rb", "-C", "/etc/foreman/dynflow/" + config},
		Env:       foremanEnv(f, 9),
		Resources: res,
	}
	d := foremanDeployment(f, component, replicas, foremanPodSpec(f, images, caps, ctr), configChecksum)
	if component == componentDynflowOrchestr {
		// Two orchestrators must never run at the same time.
		d.Spec.Strategy = appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}
	}
	return d
}

func foremanWebService(f *foremanv1alpha1.Foreman) *corev1.Service {
	svc := &corev1.Service{}
	svc.Name = foremanWebServiceName(f)
	svc.Namespace = f.Namespace
	svc.Labels = commonLabels(foremanAppName, f.Name, componentWeb)
	svc.Spec = corev1.ServiceSpec{
		Selector: selectorLabels(foremanAppName, f.Name, componentWeb),
		Ports: []corev1.ServicePort{{
			Name:       "http",
			Port:       foremanWebPort,
			TargetPort: intstr.FromString("http"),
		}},
	}
	return svc
}

// migrationJob migrates and seeds the database and runs the upgrade tasks for one hash.
func migrationJob(f *foremanv1alpha1.Foreman, images Images, caps Capabilities, hash string) *batchv1.Job {
	secrets := foremanSecretsName(f)
	ctr := corev1.Container{
		Name:    "migrate",
		Command: []string{"/bin/bash", "-c", "bin/rails db:migrate && bin/rails db:seed && bin/rails upgrade:run"},
		Env: append(foremanEnv(f, 5),
			corev1.EnvVar{Name: "SEED_ADMIN_USER", Value: "admin"},
			secretEnv("SEED_ADMIN_PASSWORD", secrets, keyAdminPassword),
		),
		Resources: defaultResources(corev1.ResourceRequirements{}, "200m", "512Mi", "2Gi"),
	}
	podSpec := foremanPodSpec(f, images, caps, ctr)
	podSpec.RestartPolicy = corev1.RestartPolicyNever

	labels := commonLabels(foremanAppName, f.Name, componentMigration)
	job := &batchv1.Job{}
	job.Name = migrationJobName(f, hash)
	job.Namespace = f.Namespace
	job.Labels = labels
	job.Spec = batchv1.JobSpec{
		BackoffLimit: new(int32(3)),
		Template: corev1.PodTemplateSpec{
			Spec: podSpec,
		},
	}
	job.Spec.Template.Labels = labels
	return job
}

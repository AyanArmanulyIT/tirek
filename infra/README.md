# Tirek AWS infrastructure

ECS task definitions are kept here as versioned templates. For the MVP the
AWS console/CloudFormation is used to create the surrounding resources
(VPC, RDS, ElastiCache, ALB, ECR, Secrets Manager); this directory holds the
reproducible application-level definitions.

## Resources per environment (staging / production)

| Resource | Name pattern | Notes |
|---|---|---|
| ECR repos | `tirek-api`, `tirek-worker`, `tirek-web` | immutable tags `sha-<commit>` |
| ECS cluster | `tirek-<env>` | Fargate, no EC2 nodes |
| ECS services | `tirek-api`, `tirek-worker`, `tirek-web` | api/web autoscale 1..3, worker fixed 1 |
| Task role | `tirek-<env>-task-role` | least-privilege: SecretsManager read, S3 bucket-scoped, KMS decrypt |
| ALB | `tirek-<env>-alb` | HTTPS cert (ACM), WAF rule set |
| RDS | `tirek-<env>-db` | postgres 16, Multi-AZ (prod), PITR 7d |
| ElastiCache | `tirek-<env>-redis` | redis 7, small + replica (prod) |
| S3 | `tirek-assets-<env>` | versioning on, private, pre-signed URL access |
| Secrets Manager | `tirek/<env>/*` | DATABASE_URL, JWT secrets, provider keys, webhook secrets |
| CloudWatch / OTel | `tirek-<env>` log groups | api, worker, web |

## Manual runbook (one-time bootstrap)

1. Create VPC (3 AZs, public + private subnets) and ECR repos.
2. Create RDS + ElastiCache + S3 + Secrets Manager entries; attach to the
   task role policy.
3. Register the task definitions below (substitute image tags).
4. Create the ALB + target groups + ACM certificate + WAF association.
5. Point GitHub Actions secrets (`AWS_ACCOUNT_ID`, `AWS_OIDC_ROLE_ARN`,
   subnet/SG ids) at the created resources.
6. First deploy via `deploy.yml`; verify `/v1/health` on both target groups.

## task-definition.api.json

```json
{
  "family": "tirek-api",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024",
  "executionRoleArn": "arn:aws:iam::<ACCOUNT>:role/tirek-exec-role",
  "taskRoleArn": "arn:aws:iam::<ACCOUNT>:role/tirek-<env>-task-role",
  "containerDefinitions": [
    {
      "name": "api",
      "image": "<ACCOUNT>.dkr.ecr.eu-central-1.amazonaws.com/tirek-api:<TAG>",
      "portMappings": [{ "containerPort": 8080, "protocol": "tcp" }],
      "secrets": [
        { "name": "DATABASE_URL", "valueFrom": "arn:aws:secretsmanager:eu-central-1:<ACCOUNT>:secret:tirek/<env>/DATABASE_URL" },
        { "name": "JWT_ACCESS_SECRET", "valueFrom": "arn:aws:secretsmanager:eu-central-1:<ACCOUNT>:secret:tirek/<env>/JWT_ACCESS_SECRET" },
        { "name": "JWT_REFRESH_SECRET", "valueFrom": "arn:aws:secretsmanager:eu-central-1:<ACCOUNT>:secret:tirek/<env>/JWT_REFRESH_SECRET" },
        { "name": "KASPI_SIGNATURE_KEY", "valueFrom": "arn:aws:secretsmanager:eu-central-1:<ACCOUNT>:secret:tirek/<env>/KASPI_SIGNATURE_KEY" }
      ],
      "environment": [
        { "name": "APP_ENV", "value": "<env>" },
        { "name": "REDIS_URL", "value": "redis://tirek-<env>-redis.xxx.ng.0001.euc1.cache.amazonaws.com:6379/0" },
        { "name": "WEB_ORIGIN", "value": "https://<env-domain>" },
        { "name": "PAYMENT_PROVIDER", "value": "<kaspi|mock>" },
        { "name": "PAYMENT_MODE", "value": "<sandbox|live>" }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": { "awslogs-group": "/ecs/tirek-<env>/api", "awslogs-region": "eu-central-1", "awslogs-stream-prefix": "api" }
      },
      "healthCheck": { "command": ["CMD-SHELL", "wget -qO- http://localhost:8080/v1/health || exit 1"] }
    }
  ]
}
```

`tirek-worker` uses the same container image with a different command
(`/app/worker`), no port mappings, and a `tirek-migrate` one-off task runs
`/app/migrate /app/migrations` before deployments.

## Guardrails

- DB and Redis are in private subnets — no public endpoints.
- Task roles never have `*` IAM permissions; S3 access is bucket-prefixed.
- Secrets are injected at task start from Secrets Manager; not baked into
  images or env.
- Every production change goes through `deploy.yml` (staging first,
  production has a manual approval gate).
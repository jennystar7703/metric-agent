import os
import requests
import json
from flask import Flask, request, logging

app = Flask(__name__)

if not app.debug:
    import logging
    from logging import StreamHandler
    handler = StreamHandler()
    handler.setLevel(logging.INFO)
    app.logger.addHandler(handler)
    app.logger.setLevel(logging.INFO)

BACKEND_URL = os.environ.get("BACKEND_URL")

def _find_attribute(attributes, key):
    for attr in attributes:
        if attr.get('key') == key:
            return next((v for k, v in attr.get('value', {}).items()), None)
    return None

@app.route('/translate', methods=['POST'])
def translate_telemetry():
    try:
        otel_data = request.json
        simple_json = {}
        
        gpu_usages = []
        gpu_temps = []
        gpu_vrams = []
        ssd_healths = []

        resource_metrics = otel_data.get('resourceMetrics', [])
        if not resource_metrics: raise ValueError("Payload missing 'resourceMetrics'")

        resource = resource_metrics[0].get('resource', {})
        for attr in resource.get('attributes', []):
            if attr.get('key') == 'host.id':
                simple_json['node_id'] = attr.get('value', {}).get('stringValue')
                break

        scope_metrics = resource_metrics[0].get('scopeMetrics', [])
        if not scope_metrics: raise ValueError("Payload missing 'scopeMetrics'")
        
        metrics = scope_metrics[0].get('metrics', [])
        for metric in metrics:
            metric_name = metric.get('name')
            data_points = metric.get('gauge', {}).get('dataPoints', [])
            
            for data_point in data_points:
                value = data_point.get('asInt') or data_point.get('asDouble')
                if value is None: continue

                if metric_name == 'system.cpu.utilization':
                    simple_json['cpu_usage_percent'] = f"{value:.1f}"
                elif metric_name == 'system.memory.utilization':
                    simple_json['mem_usage_percent'] = f"{value:.1f}"
                elif metric_name == 'system.storage.used_gb':
                    simple_json['used_storage_gb'] = str(int(value))
                # --- NEW ---: Handle the new hard disk percentage metric
                elif metric_name == 'system.harddisk.used_percent':
                    simple_json['harddisk_used_percent'] = f"{value:.1f}"
                # --- END NEW ---
                elif metric_name == 'system.gpu.utilization':
                    gpu_usages.append(value)
                elif metric_name == 'system.gpu.temperature':
                    gpu_temps.append(value)
                elif metric_name == 'system.gpu.vram.utilization':
                    gpu_vrams.append(value)
                elif metric_name == 'system.ssd.health_percent':
                    ssd_healths.append(value)

        if gpu_usages:
            avg_usage = sum(gpu_usages) / len(gpu_usages)
            simple_json['gpu_usage_percent'] = f"{avg_usage:.1f}"
        if gpu_temps:
            avg_temp = sum(gpu_temps) / len(gpu_temps)
            simple_json['gpu_temp'] = f"{avg_temp:.1f}"
        if gpu_vrams:
            avg_vram = sum(gpu_vrams) / len(gpu_vrams)
            simple_json['gpu_vram_percent'] = f"{avg_vram:.1f}"
        if ssd_healths:
            avg_health = sum(ssd_healths) / len(ssd_healths)
            simple_json['ssd_health_percent'] = f"{avg_health:.1f}"

        if 'node_id' not in simple_json: raise ValueError("Could not find node_id")

        app.logger.info(f"SUCCESS: Forwarding simple JSON to backend: {json.dumps(simple_json)}")
        
        response = requests.post(BACKEND_URL, json=simple_json, timeout=10)
        response.raise_for_status()

    except Exception as e:
        app.logger.error("Exception occurred during translation", exc_info=True)
        return "Error during transformation", 500

    return "OK", 200

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5001)
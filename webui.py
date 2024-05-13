#!/usr/bin/env python3
"""Basic status and configuration application"""
from datetime import datetime, timedelta
import gzip
import json
import os
import platform
import psutil
import secrets
import subprocess
import re
from flask import Flask, render_template, session, redirect, url_for, request, flash, jsonify, send_from_directory

app = Flask(__name__)
app.secret_key = secrets.token_hex(16)
app.permanent_session_lifetime = timedelta(minutes=10)

@app.route('/')
def index():
    session.permanent = True  # This ensures the session timeout is refreshed
    if not session.get('logged_in') or not session.get('username'):
        return render_template('login.html')
    return render_template('main.html', username=session.get('username'))

@app.route('/login', methods=['POST'])
def login():
    session.permanent = True  # Also make sure to refresh the session on login
    username = request.form['username']
    password = request.form['password']
    if username == 'admin' and password == 'admin':
        session['logged_in'] = True
        session['username'] = username
    else:
        flash('Login failed! Incorrect username or password.')
    return redirect(url_for('index'))

@app.route('/logout')
def logout():
    session.pop('logged_in', None)
    session.pop('username', None)

    return redirect(url_for('index'))

@app.route('/keepalive')
def keep_alive():
    session.modified = True
    return "Session is kept alive", 200

def translate_cores(num):
    """Translate the number of CPU cores into its corresponding name."""
    names = {
        1: "single-core",
        2: "dual-core",
        4: "quad-core",
        8: "octa-core",
        16: "hexa-core",
        32: "deca-core",
        64: "dodeca-core",
    }
    return names.get(num, f"{num}-core")

def get_cpu_info():
    """Get CPU make, model, and number of cores."""
    try:
        make = platform.processor()
        model = subprocess.check_output("cat /proc/cpuinfo | grep 'model name' | uniq | awk -F':' '{print $2}'", shell=True).decode().strip()
        cores = psutil.cpu_count(logical=False)
        return make, model, cores
    except Exception as e:
        return "N/A", "N/A", "N/A"

def get_cpu_temperature():
    """Get CPU temperature from /sys/class/hwmon/hwmon1/temp1_input."""
    try:
        with open('/sys/class/hwmon/hwmon1/temp1_input', 'r') as f:
            temp = f.readline().strip()
            # Convert temperature from millidegrees Celsius to degrees Celsius
            temp_celsius = int(temp) / 1000.0
            return f"{temp_celsius:.2f}°C"
    except FileNotFoundError:
        return "N/A"
    except Exception as e:
        return f"Error: {e}"

def sort_interfaces(iface):
    """Sort key function that places 'lo' first and sorts other interfaces naturally."""
    ifname = iface['ifname']
    if ifname == 'lo':
        return (0,)  # Return a tuple with a single integer element to prioritize 'lo'
    # Split the interface name into parts and transform digits into integers
    parts = re.split('([0-9]+)', ifname)
    return (1,) + tuple(int(part) if part.isdigit() else part.lower() for part in parts)

def format_size(bytes):
    """Format bytes to the appropriate size in KB, MB, GB, or TB."""
    kilobytes = bytes / 1024
    megabytes = kilobytes / 1024
    gigabytes = megabytes / 1024
    terabytes = gigabytes / 1024

    if gigabytes >= 1000:
        return f"{terabytes:.2f} TB"
    elif megabytes >= 1000:
        return f"{gigabytes:.2f} GB"
    elif kilobytes >= 1000:
        return f"{megabytes:.2f} MB"
    elif kilobytes >= 1:
        return f"{kilobytes:.2f} KB"
    else:
        return f"{bytes} B"

@app.route('/status')
def status():
    if not session.get('logged_in'):
        return redirect(url_for('index'))

    # Basic system information
    hostname = platform.node()
    model = subprocess.getoutput("cat /sys/devices/virtual/dmi/id/product_family")
    _, make, cores = get_cpu_info()
    cpu_chipset = f"{make}"
    #cpu_chipset = f"{psutil.cpu_count(logical=False)}-core {platform.processor()}"
    cpu_freq = psutil.cpu_freq()
    if cpu_freq:
        cpu_frequency = cpu_freq.current
        if cpu_frequency > 1000.0:
            cpu_frequency = f"{cpu_frequency / 1000:.2f} GHz ({translate_cores(cores)})"
        elif cpu_frequency < cpu_freq.min:
            cpu_frequency = f"{cpu_frequency:.2f} GHz ({translate_cores(cores)})"
        else:
            cpu_frequency = f"{cpu_frequency:.2f} MHz ({translate_cores(cores)})"
    else:
        cpu_frequency = "N/A"

    # Memory and disk usage
    memory = psutil.virtual_memory()
    disk = psutil.disk_usage('/')
    memory_data = {
        'formatted': f"{format_size(memory.used)} / {format_size(memory.total)} ({memory.percent:.2f}%)",
        'percent': memory.percent  # Raw percentage for progress bar
    }
    disk_data = {
        'formatted': f"{format_size(disk.used)} / {format_size(disk.total)} ({disk.percent:.2f}%)",
        'percent': disk.percent  # Raw percentage for progress bar
    }

    # CPU and memory usage
    cpu_usage = psutil.cpu_percent(interval=1)
    load_average = os.getloadavg()  # 1, 5, 15 minutes load averages
    load_average = tuple(map(lambda x: f"{x:.2f}", load_average))

    # Temperature (this depends highly on your system sensors setup)
    #psutil.sensors_temperatures()['coretemp'][0].current + "°C"
    try:
        cpu_temperature = get_cpu_temperature()
    except:
        cpu_temperature = "N/A"
    
    # Formatting time and uptime
    current_time = datetime.now().strftime("%a, %d %b %Y %H:%M:%S%z")
    uptime_seconds = psutil.boot_time()
    uptime = datetime.now() - datetime.fromtimestamp(uptime_seconds)
    formatted_uptime = f"{uptime.days} days {uptime.seconds // 3600} hours {(uptime.seconds // 60) % 60} mins {uptime.seconds % 60} secs"

    # Read /etc/os-release for version info
    version_info = {}
    try:
        with open('/etc/os-release') as f:
            lines = f.readlines()
            for line in lines:
                match = re.match(r'(\w+)=["\']?(.*?)["\']?$', line)
                if match:
                    version_info[match.group(1)] = match.group(2)
    except Exception as e:
        version_info['ERROR'] = str(e)

    # Get IP addresses and interface status
    cmd = "ip -j -br addr"
    result = subprocess.run(cmd.split(), stdout=subprocess.PIPE)
    interfaces = json.loads(result.stdout.decode('utf-8'))
    interfaces.sort(key=sort_interfaces)

    return render_template('status.html',
                           hostname=hostname, model=model, cpu_chipset=cpu_chipset,
                           cpu_frequency=f"{cpu_frequency}", memory=memory_data, disk=disk_data,
                           cpu_usage=cpu_usage, load_average=load_average,
                           cpu_temperature=f"{cpu_temperature}", current_time=current_time,
                           uptime=formatted_uptime, version_info=version_info, interfaces=interfaces)

@app.route('/config')
def config():
    if not session.get('logged_in'):
        return redirect(url_for('index'))

    return render_template('config.html')

@app.route('/log')
def log():
    if not session.get('logged_in'):
        return redirect(url_for('index'))

    return render_template('log.html')

@app.route('/logs')
def list_logs():
    log_dir = '/var/log'
    logs = [file for file in os.listdir(log_dir) if os.path.isfile(os.path.join(log_dir, file))]
    logs.sort()
    return jsonify(logs)

@app.route('/logs/<filename>')
def get_log(filename):
    file_path = os.path.join('/var/log', filename)
    if filename.endswith('.gz'):
        with gzip.open(file_path, 'rt') as f:
            content = f.read()
        return content
    return send_from_directory('/var/log', filename)

@app.route('/upgrade')
def upgrade():
    if not session.get('logged_in'):
        return redirect(url_for('index'))

    return render_template('upgrade.html')

if __name__ == '__main__':
    app.run(debug=True)

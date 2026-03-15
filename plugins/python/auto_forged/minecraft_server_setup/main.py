import sys
import json
import os
import subprocess
import urllib.request
import shutil

def run_bash(command):
    """Execute bash command and return output."""
    try:
        result = subprocess.run(
            command, shell=True, capture_output=True, text=True, timeout=300
        )
        return {
            "success": result.returncode == 0,
            "output": result.stdout,
            "error": result.stderr
        }
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def download_file(url, output_path):
    """Download file from URL to output path."""
    try:
        urllib.request.urlretrieve(url, output_path)
        return {"success": True, "output": f"Downloaded {url} to {output_path}", "error": ""}
    except Exception as e:
        return {"success": False, "output": "", "error": str(e)}

def install_jdk():
    """Install OpenJDK via package manager."""
    cmds = [
        "apt-get update && apt-get install -y openjdk-17-jdk",
        "java -version"
    ]
    results = []
    for cmd in cmds:
        results.append(run_bash(cmd))
    return results

def setup_fabric_server(mc_version, loader_version, output_dir):
    """Download and install Fabric server."""
    fabric_url = f"https://maven.fabricmc.net/net/fabricmc/fabric-installer/{loader_version}/fabric-installer-{loader_version}.jar"
    installer_path = f"/tmp/fabric-installer-{loader_version}.jar"
    
    # Download installer
    dl_result = download_file(fabric_url, installer_path)
    if not dl_result["success"]:
        return dl_result
    
    # Run installer
    install_cmd = f"java -jar {installer_path} server -mcversion {mc_version} -downloadMinecraft -dir {output_dir}"
    return run_bash(install_cmd)

def setup_forge_server(mc_version, forge_version, output_dir):
    """Download and install Forge server."""
    forge_url = f"https://maven.minecraftforge.net/net/minecraftforge/forge/{mc_version}-{forge_version}/forge-{mc_version}-{forge_version}-installer.jar"
    installer_path = f"/tmp/forge-installer-{mc_version}-{forge_version}.jar"
    
    # Download installer
    dl_result = download_file(forge_url, installer_path)
    if not dl_result["success"]:
        return dl_result
    
    # Run installer
    install_cmd = f"java -jar {installer_path} --installServer -d {output_dir}"
    return run_bash(install_cmd)

def start_minecraft_server(jar_path, memory_gb, nogui=True):
    """Start Minecraft server in background."""
    cmd = f"java -Xmx{memory_gb}G -jar {jar_path} {'nogui' if nogui else ''}"
    return run_bash(cmd)

def create_mods_directory(base_dir):
    """Create mods directory for server."""
    mods_dir = os.path.join(base_dir, "mods")
    plugins_dir = os.path.join(base_dir, "plugins")
    
    os.makedirs(mods_dir, exist_ok=True)
    os.makedirs(plugins_dir, exist_ok=True)
    
    return {"success": True, "output": f"Created {mods_dir} and {plugins_dir}", "error": ""}

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError as e:
        print(json.dumps({"success": False, "output": "", "error": f"Invalid JSON input: {str(e)}"}))
        return
    
    action = input_data.get("action", "")
    
    if action == "install_jdk":
        result = install_jdk()
        combined = {"success": all(r["success"] for r in result), "output": str(result), "error": ""}
    
    elif action == "setup_fabric":
        mc_version = input_data.get("mc_version", "1.20.1")
        loader_version = input_data.get("loader_version", "0.15.11")
        output_dir = input_data.get("output_dir", "/tmp/fabric_server")
        result = setup_fabric_server(mc_version, loader_version, output_dir)
        combined = result
    
    elif action == "setup_forge":
        mc_version = input_data.get("mc_version", "1.20.1")
        forge_version = input_data.get("forge_version", "47.2.0")
        output_dir = input_data.get("output_dir", "/tmp/forge_server")
        result = setup_forge_server(mc_version, forge_version, output_dir)
        combined = result
    
    elif action == "start_server":
        jar_path = input_data.get("jar_path", "server.jar")
        memory_gb = input_data.get("memory_gb", 4)
        result = start_minecraft_server(jar_path, memory_gb)
        combined = result
    
    elif action == "create_dirs":
        base_dir = input_data.get("base_dir", "/tmp/mc_server")
        combined = create_mods_directory(base_dir)
    
    elif action == "download":
        url = input_data.get("url", "")
        output = input_data.get("output", "/tmp/downloaded.file")
        if not url:
            combined = {"success": False, "output": "", "error": "Missing URL"}
        else:
            combined = download_file(url, output)
    
    elif action == "bash":
        command = input_data.get("command", "")
        if not command:
            combined = {"success": False, "output": "", "error": "Missing command"}
        else:
            combined = run_bash(command)
    
    else:
        combined = {"success": False, "output": "", "error": f"Unknown action: {action}"}
    
    print(json.dumps(combined))

if __name__ == "__main__":
    main()

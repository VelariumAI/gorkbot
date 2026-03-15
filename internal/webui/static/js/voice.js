export class IntelligentVoiceRouter {
  constructor() {
    this.recognition = null;
    this.synth = window.speechSynthesis;
    this.isListening = false;
    this.init();
  }

  init() {
    const SpeechRecognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SpeechRecognition) {
      console.warn("Web Speech API not supported in this environment.");
      return;
    }
    this.recognition = new SpeechRecognition();
    this.recognition.continuous = false;
    this.recognition.interimResults = false;
    this.recognition.lang = 'en-US';

    this.recognition.onresult = (event) => this.handleResult(event);
    this.recognition.onerror = (event) => this.handleError(event);
    this.recognition.onend = () => { this.isListening = false; };
  }

  startListening() {
    if (this.recognition && !this.isListening) {
      this.recognition.start();
      this.isListening = true;
      this.speak("Listening.");
    }
  }

  handleResult(event) {
    const result = event.results[0][0];
    const transcript = result.transcript.toLowerCase();
    const confidence = result.confidence;

    if (confidence < 0.8) {
      this.speak("Command unclear, added to text input for clarification.");
      const input = document.getElementById('promptInput');
      if(input) input.value = transcript;
      return;
    }

    this.routeCommand(transcript);
  }

  routeCommand(cmd) {
    if (cmd.includes('execute') || cmd.includes('run tool')) {
      this.speak("Executing tool.");
      window.dispatchEvent(new CustomEvent('VOICE_COMMAND', { detail: { action: 'execute', raw: cmd } }));
    } else if (cmd.includes('clear') || cmd.includes('reset')) {
      this.speak("Clearing input.");
      const input = document.getElementById('promptInput');
      if(input) input.value = '';
    } else {
      const input = document.getElementById('promptInput');
      if(input) input.value = cmd;
      this.speak("Input updated.");
    }
  }

  speak(text) {
    if (!this.synth) {
      this.showToast(text);
      return;
    }
    const utterThis = new SpeechSynthesisUtterance(text);
    this.synth.speak(utterThis);
  }

  showToast(msg) {
    console.log("Voice Feedback: " + msg);
  }

  handleError(event) {
    console.error("Speech Recognition Error: ", event.error);
    this.isListening = false;
  }
}

#!/usr/bin/env python3
"""
Example AI Analysis Tool for Gorkbot

This demonstrates how to use Python ML/NLP libraries with Gorkbot.
Shows sentiment analysis, entity extraction, and text summarization.
"""

import sys
import os

# Add parent directory to path for gorkbot_bridge import
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from gorkbot_bridge import tool, ToolResult, run

# Lazy import for optional dependencies
_textblob = None
_nltk = None


def _get_textblob():
    global _textblob
    if _textblob is None:
        try:
            from textblob import TextBlob
            _textblob = TextBlob
        except ImportError:
            return None
    return _textblob


def _get_nltk():
    global _nltk
    if _nltk is None:
        try:
            import nltk
            _nltk = nltk
        except ImportError:
            return None
    return _nltk


@tool(description="Analyze sentiment of text")
def sentiment(text: str) -> ToolResult:
    """
    Analyze the sentiment of the given text.

    Returns polarity (-1 to 1) and subjectivity (0 to 1).
    """
    tb = _get_textblob()
    if tb is None:
        return ToolResult(
            success=False,
            error="textblob not installed. Run: pip install textblob"
        )

    try:
        blob = tb(text)
        polarity = blob.sentiment.polarity
        subjectivity = blob.sentiment.subjectivity

        # Interpret results
        if polarity > 0.1:
            sentiment_label = "POSITIVE"
        elif polarity < -0.1:
            sentiment_label = "NEGATIVE"
        else:
            sentiment_label = "NEUTRAL"

        if subjectivity > 0.6:
            opinion_label = "OPINION"
        elif subjectivity < 0.4:
            opinion_label = "FACT"
        else:
            opinion_label = "MIXED"

        output = f"Sentiment Analysis Results:\n"
        output += f"  Sentiment: {sentiment_label}\n"
        output += f"  Polarity: {polarity:.3f} (-1 to +1)\n"
        output += f"  Subjectivity: {subjectivity:.3f} (0 to 1)\n"
        output += f"  Interpretation: {opinion_label}"

        return ToolResult(
            success=True,
            output=output,
            data={
                "polarity": polarity,
                "subjectivity": subjectivity,
                "sentiment": sentiment_label,
                "interpretation": opinion_label
            }
        )
    except Exception as e:
        return ToolResult(success=False, error=str(e))


@tool(description="Extract named entities from text")
def entities(text: str) -> ToolResult:
    """
    Extract named entities (people, organizations, locations) from text.
    """
    nltk = _get_nltk()
    if nltk is None:
        return ToolResult(
            success=False,
            error="nltk not installed. Run: pip install nltk"
        )

    try:
        # Download required NLTK data if needed
        try:
            nltk.data.find('tokenizers/punkt')
        except LookupError:
            nltk.download('punkt', quiet=True)

        try:
            nltk.data.find('taggers/averaged_perceptron_tagger')
        except LookupError:
            nltk.download('averaged_perceptron_tagger', quiet=True)

        try:
            nltk.data.find('chunkers/maxent_ne_chunker')
        except LookupError:
            nltk.download('maxent_ne_chunker', quiet=True)

        try:
            nltk.data.find('corpora/words')
        except LookupError:
            nltk.download('words', quiet=True)

        # Do named entity recognition
        sentences = nltk.sent_tokenize(text)
        tokenized_sentences = [nltk.word_tokenize(sentence) for sentence in sentences]
        tagged_sentences = [nltk.pos_tag(sentence) for sentence in tokenized_sentences]
        named_entities = nltk.ne_chunk_sentences(tagged_sentences)

        # Extract entities by type
        people = []
        organizations = []
        locations = []

        for tree in named_entities:
            if hasattr(tree, 'label'):
                entity = ' '.join([c[0] for c in tree.leaves()])
                if tree.label() == 'PERSON':
                    people.append(entity)
                elif tree.label() == 'ORGANIZATION':
                    organizations.append(entity)
                elif tree.label() == 'GPE':
                    locations.append(entity)

        output = "Named Entity Recognition Results:\n"

        if people:
            output += f"  People: {', '.join(people)}\n"
        if organizations:
            output += f"  Organizations: {', '.join(organizations)}\n"
        if locations:
            output += f"  Locations: {', '.join(locations)}\n"

        if not (people or organizations or locations):
            output += "  No named entities found.\n"

        return ToolResult(
            success=True,
            output=output,
            data={
                "people": people,
                "organizations": organizations,
                "locations": locations
            }
        )

    except Exception as e:
        return ToolResult(success=False, error=str(e))


@tool(description="Extract keywords from text")
def keywords(text: str, top_n: int = 10) -> ToolResult:
    """
    Extract the most important keywords from text using TF-IDF-like approach.
    """
    tb = _get_textblob()
    if tb is None:
        return ToolResult(
            success=False,
            error="textblob not installed"
        )

    try:
        blob = tb(text)

        # Extract noun phrases as keywords
        noun_phrases = blob.noun_phrases

        # Count frequency
        from collections import Counter
        phrase_counts = Counter(noun_phrases)

        # Get top N
        top_keywords = phrase_counts.most_common(top_n)

        output = f"Top {top_n} Keywords:\n"
        for i, (phrase, count) in enumerate(top_keywords, 1):
            output += f"  {i}. {phrase} ({count})\n"

        return ToolResult(
            success=True,
            output=output,
            data={"keywords": [{"phrase": k, "count": c} for k, c in top_keywords]}
        )

    except Exception as e:
        return ToolResult(success=False, error=str(e))


@tool(description="Generate text summary")
def summarize(text: str, sentences: int = 3) -> ToolResult:
    """
    Generate a brief summary of the text.
    Uses extractive summarization based on sentence scoring.
    """
    tb = _get_textblob()
    if tb is None:
        return ToolResult(success=False, error="textblob not installed")

    try:
        blob = tb(text)
        sentence_list = blob.sentences

        if len(sentence_list) <= sentences:
            # Text is short enough, return as-is
            output = "Summary (text is short):\n" + text
            return ToolResult(
                success=True,
                output=output,
                data={"summary": text, "method": "original"}
            )

        # Score sentences by word count and polarity variance
        scored_sentences = []
        for i, sent in enumerate(sentence_list):
            score = len(sent.words)  # Prefer longer sentences
            score += abs(sent.sentiment.polarity)  # Prefer expressive sentences
            scored_sentences.append((score, i, str(sent)))

        # Sort by score and get top N
        scored_sentences.sort(reverse=True)
        selected_indices = [idx for _, idx, _ in scored_sentences[:sentences]]
        selected_indices.sort()  # Restore original order

        summary = ' '.join([str(sentence_list[i]) for i in selected_indices])

        output = f"Summary ({sentences} sentences):\n{summary}"

        return ToolResult(
            success=True,
            output=output,
            data={"summary": summary, "method": "extractive", "sentences": sentences}
        )

    except Exception as e:
        return ToolResult(success=False, error=str(e))


@tool(description="Perform full AI analysis on text")
def analyze(text: str, analysis_type: str = "sentiment") -> ToolResult:
    """
    Main analysis function - routes to specific analysis type.

    Supported types:
    - sentiment: Overall sentiment analysis
    - entities: Named entity recognition
    - keywords: Key phrase extraction
    - summary: Text summarization
    - full: All of the above
    """
    if analysis_type == "sentiment":
        return sentiment(text)
    elif analysis_type == "entities":
        return entities(text)
    elif analysis_type == "keywords":
        return keywords(text)
    elif analysis_type == "summary":
        return summarize(text)
    elif analysis_type == "full":
        # Run all analyses
        sent_result = sentiment(text)
        ent_result = entities(text)
        kw_result = keywords(text)
        sum_result = summarize(text)

        output = "Full AI Analysis Report:\n"
        output += "=" * 40 + "\n\n"

        if sent_result.success:
            output += "SENTIMENT:\n" + sent_result.output + "\n\n"
        if ent_result.success:
            output += "ENTITIES:\n" + ent_result.output + "\n\n"
        if kw_result.success:
            output += "KEYWORDS:\n" + kw_result.output + "\n\n"
        if sum_result.success:
            output += "SUMMARY:\n" + sum_result.output + "\n\n"

        return ToolResult(
            success=True,
            output=output,
            data={
                "sentiment": sent_result.data if sent_result.success else None,
                "entities": ent_result.data if ent_result.success else None,
                "keywords": kw_result.data if kw_result.success else None,
                "summary": sum_result.data if sum_result.success else None
            }
        )
    else:
        return ToolResult(
            success=False,
            error=f"Unknown analysis type: {analysis_type}. "
                 "Supported: sentiment, entities, keywords, summary, full"
        )


# Main entry point
if __name__ == "__main__":
    run()
